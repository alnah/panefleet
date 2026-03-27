package panes

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alnah/panefleet/internal/state"
)

func TestServiceOverrideMethods(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	at := time.Now().UTC().Add(-1 * time.Second)

	if _, err := svc.Ingest(ctx, state.Event{
		PaneID:     "%61",
		Kind:       state.EventPaneStarted,
		OccurredAt: at,
	}); err != nil {
		t.Fatalf("ingest start: %v", err)
	}

	if _, err := svc.SetOverride(ctx, "%61", state.StatusStale, "test"); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	out, err := svc.StateShow(ctx, "%61")
	if err != nil {
		t.Fatalf("StateShow: %v", err)
	}
	if out.Status != state.StatusStale {
		t.Fatalf("override status = %s, want STALE", out.Status)
	}

	if _, err := svc.ClearOverride(ctx, "%61", "test"); err != nil {
		t.Fatalf("ClearOverride: %v", err)
	}
}

func TestServiceClearOverrideRestoresUnderlyingState(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-2 * time.Second)

	if _, err := svc.Ingest(ctx, state.Event{
		PaneID:     "%62",
		Kind:       state.EventPaneStarted,
		OccurredAt: base,
		Source:     "adapter:test",
	}); err != nil {
		t.Fatalf("ingest start: %v", err)
	}
	if _, err := svc.SetOverride(ctx, "%62", state.StatusStale, "test"); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}

	exitCode := 2
	exitAt := time.Now().UTC()
	shown, err := svc.Ingest(ctx, state.Event{
		PaneID:     "%62",
		Kind:       state.EventPaneExited,
		OccurredAt: exitAt,
		ExitCode:   &exitCode,
		Source:     "adapter:test",
	})
	if err != nil {
		t.Fatalf("ingest exit under override: %v", err)
	}
	if shown.Status != state.StatusStale {
		t.Fatalf("override should still mask state, got %s", shown.Status)
	}

	if _, err := svc.ClearOverride(ctx, "%62", "test"); err != nil {
		t.Fatalf("ClearOverride: %v", err)
	}
	got, err := svc.StateShow(ctx, "%62")
	if err != nil {
		t.Fatalf("StateShow after clear: %v", err)
	}
	if got.Status != state.StatusError {
		t.Fatalf("clear override should reveal underlying ERROR, got %s", got.Status)
	}
	if got.LastExitCode == nil || *got.LastExitCode != exitCode {
		t.Fatalf("last exit code mismatch: %+v", got.LastExitCode)
	}
}

func TestServiceStateShowErrors(t *testing.T) {
	svc := newService(t)
	_, err := svc.StateShow(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "pane id is required") {
		t.Fatalf("expected pane id required, got: %v", err)
	}

	_, err = svc.StateShow(context.Background(), "%missing")
	if err == nil || !strings.Contains(err.Error(), "pane not found") {
		t.Fatalf("expected pane not found, got: %v", err)
	}
}

func TestServiceOverrideMethodsTolerateFutureLastEventAt(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	future := time.Now().UTC().Add(2 * time.Minute)

	if _, err := svc.Ingest(ctx, state.Event{
		PaneID:     "%63",
		Kind:       state.EventPaneStarted,
		OccurredAt: future,
		Source:     "adapter:test",
	}); err != nil {
		t.Fatalf("ingest future start: %v", err)
	}

	if _, err := svc.SetOverride(ctx, "%63", state.StatusStale, "test"); err != nil {
		t.Fatalf("SetOverride with future state: %v", err)
	}
	if _, err := svc.ClearOverride(ctx, "%63", "test"); err != nil {
		t.Fatalf("ClearOverride with future state: %v", err)
	}
}

type overrideConcurrencyStore struct {
	mu          sync.Mutex
	state       state.PaneState
	hasState    bool
	inFlight    int32
	maxInFlight int32
}

func (s *overrideConcurrencyStore) Init(context.Context) error { return nil }
func (s *overrideConcurrencyStore) Close() error               { return nil }

func (s *overrideConcurrencyStore) GetPaneState(_ context.Context, _ string) (state.PaneState, bool, error) {
	s.enter()
	defer s.leave()

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasState {
		return state.PaneState{}, false, nil
	}
	return s.state, true, nil
}

func (s *overrideConcurrencyStore) AppendAndProject(_ context.Context, _ state.Event, st state.PaneState) error {
	s.enter()
	defer s.leave()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = st
	s.hasState = true
	return nil
}

func (s *overrideConcurrencyStore) ListPaneStates(context.Context) ([]state.PaneState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasState {
		return nil, nil
	}
	return []state.PaneState{s.state}, nil
}

func (s *overrideConcurrencyStore) enter() {
	current := atomic.AddInt32(&s.inFlight, 1)
	for {
		maxSeen := atomic.LoadInt32(&s.maxInFlight)
		if current <= maxSeen || atomic.CompareAndSwapInt32(&s.maxInFlight, maxSeen, current) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
}

func (s *overrideConcurrencyStore) leave() {
	atomic.AddInt32(&s.inFlight, -1)
}

func TestServiceSetOverrideSerializesAgainstIngest(t *testing.T) {
	reducer, err := state.NewReducer(state.Config{
		DoneRecentWindow: 10 * time.Minute,
		StaleWindow:      45 * time.Minute,
	})
	if err != nil {
		t.Fatalf("new reducer: %v", err)
	}

	base := time.Now().UTC()
	st := &overrideConcurrencyStore{
		state: state.PaneState{
			PaneID:           "%64",
			Status:           state.StatusRun,
			StatusSource:     "adapter:test",
			ReasonCode:       "pane.started",
			Version:          1,
			LastEventAt:      base,
			LastTransitionAt: base,
		},
		hasState: true,
	}
	svc := NewService(reducer, st)

	errCh := make(chan error, 2)
	go func() {
		_, err := svc.SetOverride(context.Background(), "%64", state.StatusStale, "test")
		errCh <- err
	}()
	time.Sleep(5 * time.Millisecond)
	go func() {
		_, err := svc.Ingest(context.Background(), state.Event{
			ID:         "evt-override-race",
			PaneID:     "%64",
			Kind:       state.EventPaneWaiting,
			OccurredAt: base.Add(time.Second),
			Source:     "adapter:test",
		})
		errCh <- err
	}()

	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("service call failed: %v", err)
		}
	}
	if got := atomic.LoadInt32(&st.maxInFlight); got != 1 {
		t.Fatalf("override and ingest should serialize store access per pane, max concurrent calls=%d", got)
	}
}

func TestServiceRejectsEmptyPaneIDs(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	if _, err := svc.Ingest(ctx, state.Event{Kind: state.EventPaneStarted, OccurredAt: time.Now().UTC()}); err == nil || !strings.Contains(err.Error(), "pane id is required") {
		t.Fatalf("expected ingest pane id validation error, got %v", err)
	}
	if _, err := svc.SetOverride(ctx, "", state.StatusStale, "test"); err == nil || !strings.Contains(err.Error(), "pane id is required") {
		t.Fatalf("expected set override pane id validation error, got %v", err)
	}
	if _, err := svc.ClearOverride(ctx, "", "test"); err == nil || !strings.Contains(err.Error(), "pane id is required") {
		t.Fatalf("expected clear override pane id validation error, got %v", err)
	}
}
