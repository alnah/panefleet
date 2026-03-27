package app

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

type Service struct {
	reducer *state.Reducer
	store   store.Store
	mu      sync.RWMutex
	subs    map[chan state.PaneState]struct{}
}

func NewService(reducer *state.Reducer, st store.Store) *Service {
	return &Service{
		reducer: reducer,
		store:   st,
		subs:    make(map[chan state.PaneState]struct{}),
	}
}

func (s *Service) Ingest(ctx context.Context, ev state.Event) (state.PaneState, error) {
	if ev.ID == "" {
		ev.ID = uuid.NewString()
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now().UTC()
	}

	current, ok, err := s.store.GetPaneState(ctx, ev.PaneID)
	if err != nil {
		return state.PaneState{}, err
	}
	if !ok {
		current = state.NewPaneState(ev.PaneID)
		current.LastEventAt = ev.OccurredAt
		current.LastTransitionAt = ev.OccurredAt
	}

	next, err := s.reducer.Apply(current, ev)
	if err != nil {
		return state.PaneState{}, err
	}
	if err := s.store.AppendAndProject(ctx, ev, next); err != nil {
		return state.PaneState{}, err
	}
	s.publish(next)
	return next, nil
}

func (s *Service) StateShow(ctx context.Context, paneID string) (state.PaneState, error) {
	if paneID == "" {
		return state.PaneState{}, errors.New("pane id is required")
	}
	st, ok, err := s.store.GetPaneState(ctx, paneID)
	if err != nil {
		return state.PaneState{}, err
	}
	if !ok {
		return state.PaneState{}, fmt.Errorf("pane not found: %s", paneID)
	}
	return st, nil
}

func (s *Service) StateList(ctx context.Context) ([]state.PaneState, error) {
	return s.store.ListPaneStates(ctx)
}

func (s *Service) SetOverride(ctx context.Context, paneID string, target state.Status, source string) (state.PaneState, error) {
	ev := state.Event{
		PaneID:     paneID,
		Kind:       state.EventOverrideSet,
		OccurredAt: time.Now().UTC(),
		OverrideTo: target,
		Source:     source,
		ReasonCode: "override.set",
	}
	return s.Ingest(ctx, ev)
}

func (s *Service) ClearOverride(ctx context.Context, paneID, source string) (state.PaneState, error) {
	ev := state.Event{
		PaneID:     paneID,
		Kind:       state.EventOverrideCleared,
		OccurredAt: time.Now().UTC(),
		Source:     source,
		ReasonCode: "override.cleared",
	}
	return s.Ingest(ctx, ev)
}

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
