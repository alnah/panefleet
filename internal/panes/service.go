package panes

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/alnah/panefleet/internal/state"
	"github.com/alnah/panefleet/internal/store"
)

// Service coordinates reducer decisions, persistence, and fan-out updates for
// pane state consumers.
type Service struct {
	reducer *state.Reducer
	store   store.Store
	mu      sync.RWMutex
	subs    map[chan state.PaneState]struct{}
	lockMu  sync.Mutex
	locks   map[string]*paneLock
}

type paneLock struct {
	mu   sync.Mutex
	refs int
}

// NewService builds the application boundary that keeps reducer rules and
// persistence updates in one place, so callers cannot diverge business writes.
func NewService(reducer *state.Reducer, st store.Store) *Service {
	return &Service{
		reducer: reducer,
		store:   st,
		subs:    make(map[chan state.PaneState]struct{}),
		locks:   make(map[string]*paneLock),
	}
}

const maxIngestAttempts = 8

// Ingest applies one event to the canonical pane stream and persists the
// projected state atomically to keep runtime and storage aligned.
func (s *Service) Ingest(ctx context.Context, ev state.Event) (state.PaneState, error) {
	if ev.PaneID == "" {
		return state.PaneState{}, errors.New("pane id is required")
	}
	unlock := s.lockPane(ev.PaneID)
	defer unlock()
	return s.ingestLocked(ctx, ev)
}

func (s *Service) ingestLocked(ctx context.Context, ev state.Event) (state.PaneState, error) {
	if ev.ID == "" {
		ev.ID = uuid.NewString()
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now().UTC()
	}

	errScope := fmt.Sprintf("pane=%s kind=%s event_id=%s", ev.PaneID, ev.Kind, ev.ID)
	for attempt := 0; attempt < maxIngestAttempts; attempt++ {
		current, ok, err := s.store.GetPaneState(ctx, ev.PaneID)
		if err != nil {
			return state.PaneState{}, fmt.Errorf("ingest lookup (%s): %w", errScope, err)
		}
		if !ok {
			current = state.NewPaneState(ev.PaneID)
			current.LastEventAt = ev.OccurredAt
			current.LastTransitionAt = ev.OccurredAt
		}

		next, err := s.reducer.Apply(current, ev)
		if err != nil {
			return state.PaneState{}, fmt.Errorf("ingest reduce (%s): %w", errScope, err)
		}
		if err := s.store.AppendAndProject(ctx, ev, next); err != nil {
			if errors.Is(err, store.ErrConcurrentWrite) {
				continue
			}
			return state.PaneState{}, fmt.Errorf("ingest persist (%s): %w", errScope, err)
		}

		persisted, ok, err := s.store.GetPaneState(ctx, ev.PaneID)
		if err != nil {
			return state.PaneState{}, fmt.Errorf("ingest reload (%s): %w", errScope, err)
		}
		if !ok {
			return state.PaneState{}, fmt.Errorf("ingest reload (%s): pane state missing after persist", errScope)
		}
		view := persisted.Effective()
		s.publish(view)
		return view, nil
	}

	return state.PaneState{}, fmt.Errorf("ingest persist (%s): %w", errScope, store.ErrConcurrentWrite)
}

// StateShow returns one pane projection and fails fast on missing pane ids to
// make CLI/API misuse explicit.
func (s *Service) StateShow(ctx context.Context, paneID string) (state.PaneState, error) {
	if paneID == "" {
		return state.PaneState{}, errors.New("pane id is required")
	}
	st, ok, err := s.store.GetPaneState(ctx, paneID)
	if err != nil {
		return state.PaneState{}, fmt.Errorf("state-show pane=%s: %w", paneID, err)
	}
	if !ok {
		return state.PaneState{}, fmt.Errorf("pane not found: %s", paneID)
	}
	return st.Effective(), nil
}

// StateList returns the current projection set sorted by the store contract.
func (s *Service) StateList(ctx context.Context) ([]state.PaneState, error) {
	list, err := s.store.ListPaneStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("state-list: %w", err)
	}
	for i := range list {
		list[i] = list[i].Effective()
	}
	return list, nil
}

// SetOverride sets a manual status that intentionally wins over adapter events
// until cleared, for operator-controlled recovery workflows.
func (s *Service) SetOverride(ctx context.Context, paneID string, target state.Status, source string) (state.PaneState, error) {
	if paneID == "" {
		return state.PaneState{}, errors.New("pane id is required")
	}
	unlock := s.lockPane(paneID)
	defer unlock()

	occurredAt, err := s.controlOccurredAtLocked(ctx, paneID)
	if err != nil {
		return state.PaneState{}, fmt.Errorf("set-override pane=%s: %w", paneID, err)
	}
	ev := state.Event{
		PaneID:     paneID,
		Kind:       state.EventOverrideSet,
		OccurredAt: occurredAt,
		OverrideTo: target,
		Source:     source,
		ReasonCode: "override.set",
	}
	return s.ingestLocked(ctx, ev)
}

// ClearOverride removes a manual status so reducer time/event rules become
// authoritative again.
func (s *Service) ClearOverride(ctx context.Context, paneID, source string) (state.PaneState, error) {
	if paneID == "" {
		return state.PaneState{}, errors.New("pane id is required")
	}
	unlock := s.lockPane(paneID)
	defer unlock()

	occurredAt, err := s.controlOccurredAtLocked(ctx, paneID)
	if err != nil {
		return state.PaneState{}, fmt.Errorf("clear-override pane=%s: %w", paneID, err)
	}
	ev := state.Event{
		PaneID:     paneID,
		Kind:       state.EventOverrideCleared,
		OccurredAt: occurredAt,
		Source:     source,
		ReasonCode: "override.cleared",
	}
	return s.ingestLocked(ctx, ev)
}

// Subscribe exposes best-effort update notifications for UI refresh paths
// without coupling callers to storage internals.
func (s *Service) Subscribe() (<-chan state.PaneState, func()) {
	ch := make(chan state.PaneState, 64)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	cancel := func() {
		s.mu.Lock()
		if _, ok := s.subs[ch]; ok {
			delete(s.subs, ch)
			close(ch)
		}
		s.mu.Unlock()
	}
	return ch, cancel
}

func (s *Service) publish(st state.PaneState) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch := range s.subs {
		select {
		case ch <- st:
		default:
		}
	}
}

func (s *Service) lockPane(paneID string) func() {
	s.lockMu.Lock()
	lock, ok := s.locks[paneID]
	if !ok {
		lock = &paneLock{}
		s.locks[paneID] = lock
	}
	lock.refs++
	s.lockMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()

		s.lockMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(s.locks, paneID)
		}
		s.lockMu.Unlock()
	}
}

func (s *Service) controlOccurredAtLocked(ctx context.Context, paneID string) (time.Time, error) {
	now := time.Now().UTC()
	current, ok, err := s.store.GetPaneState(ctx, paneID)
	if err != nil {
		return time.Time{}, err
	}
	if ok && current.LastEventAt.After(now) {
		return current.LastEventAt, nil
	}
	return now, nil
}
