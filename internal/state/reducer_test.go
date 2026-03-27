package state

import (
	"testing"
	"time"
)

func mustReducer(t *testing.T) *Reducer {
	t.Helper()
	r, err := NewReducer(Config{
		DoneRecentWindow: 10 * time.Minute,
		StaleWindow:      45 * time.Minute,
	})
	if err != nil {
		t.Fatalf("new reducer: %v", err)
	}
	return r
}

func TestReducerLifecycle(t *testing.T) {
	r := mustReducer(t)
	paneID := "%1"
	base := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)

	state := NewPaneState(paneID)
	state.LastEventAt = base
	state.LastTransitionAt = base

	runState, err := r.Apply(state, Event{
		PaneID:     paneID,
		Kind:       EventPaneStarted,
		OccurredAt: base.Add(1 * time.Second),
		Source:     "adapter:tmux",
	})
	if err != nil {
		t.Fatalf("apply start: %v", err)
	}
	if runState.Status != StatusRun {
		t.Fatalf("want RUN, got %s", runState.Status)
	}

	exitCode := 0
	doneState, err := r.Apply(runState, Event{
		PaneID:     paneID,
		Kind:       EventPaneExited,
		OccurredAt: base.Add(2 * time.Second),
		ExitCode:   &exitCode,
		Source:     "adapter:tmux",
	})
	if err != nil {
		t.Fatalf("apply exit: %v", err)
	}
	if doneState.Status != StatusDone {
		t.Fatalf("want DONE, got %s", doneState.Status)
	}

	idleState, err := r.Apply(doneState, Event{
		PaneID:     paneID,
		Kind:       EventTimerRecompute,
		OccurredAt: base.Add(11 * time.Minute),
		Source:     "timer",
	})
	if err != nil {
		t.Fatalf("recompute done->idle: %v", err)
	}
	if idleState.Status != StatusIdle {
		t.Fatalf("want IDLE, got %s", idleState.Status)
	}

	staleState, err := r.Apply(idleState, Event{
		PaneID:     paneID,
		Kind:       EventTimerRecompute,
		OccurredAt: base.Add(57 * time.Minute),
		Source:     "timer",
	})
	if err != nil {
		t.Fatalf("recompute idle->stale: %v", err)
	}
	if staleState.Status != StatusStale {
		t.Fatalf("want STALE, got %s", staleState.Status)
	}
}

func TestReducerExitNonZeroGoesError(t *testing.T) {
	r := mustReducer(t)
	paneID := "%2"
	base := time.Date(2026, 3, 26, 11, 0, 0, 0, time.UTC)

	state := NewPaneState(paneID)
	state.LastEventAt = base
	state.LastTransitionAt = base

	started, err := r.Apply(state, Event{
		PaneID:     paneID,
		Kind:       EventPaneStarted,
		OccurredAt: base.Add(1 * time.Second),
	})
	if err != nil {
		t.Fatalf("apply start: %v", err)
	}

	code := 2
	failed, err := r.Apply(started, Event{
		PaneID:     paneID,
		Kind:       EventPaneExited,
		OccurredAt: base.Add(2 * time.Second),
		ExitCode:   &code,
	})
	if err != nil {
		t.Fatalf("apply failed exit: %v", err)
	}
	if failed.Status != StatusError {
		t.Fatalf("want ERROR, got %s", failed.Status)
	}
}

func TestReducerManualOverrideWins(t *testing.T) {
	r := mustReducer(t)
	paneID := "%3"
	base := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)

	state := NewPaneState(paneID)
	state.LastEventAt = base
	state.LastTransitionAt = base

	overrideState, err := r.Apply(state, Event{
		PaneID:     paneID,
		Kind:       EventOverrideSet,
		OccurredAt: base.Add(1 * time.Second),
		OverrideTo: StatusStale,
		Source:     "manual",
	})
	if err != nil {
		t.Fatalf("set override: %v", err)
	}
	if overrideState.Status != StatusStale {
		t.Fatalf("want STALE, got %s", overrideState.Status)
	}

	afterStart, err := r.Apply(overrideState, Event{
		PaneID:     paneID,
		Kind:       EventPaneStarted,
		OccurredAt: base.Add(2 * time.Second),
		Source:     "adapter:tmux",
	})
	if err != nil {
		t.Fatalf("apply start under override: %v", err)
	}
	if afterStart.Status != StatusStale {
		t.Fatalf("override should win, got %s", afterStart.Status)
	}

	cleared, err := r.Apply(afterStart, Event{
		PaneID:     paneID,
		Kind:       EventOverrideCleared,
		OccurredAt: base.Add(3 * time.Second),
		Source:     "manual",
	})
	if err != nil {
		t.Fatalf("clear override: %v", err)
	}
	if cleared.ManualOverride != nil {
		t.Fatalf("override should be nil")
	}
}

func TestReducerRejectsOutOfOrderEvent(t *testing.T) {
	r := mustReducer(t)
	paneID := "%4"
	base := time.Date(2026, 3, 26, 13, 0, 0, 0, time.UTC)

	state := NewPaneState(paneID)
	state.LastEventAt = base.Add(10 * time.Second)
	state.LastTransitionAt = base

	_, err := r.Apply(state, Event{
		PaneID:     paneID,
		Kind:       EventPaneStarted,
		OccurredAt: base.Add(9 * time.Second),
	})
	if err == nil {
		t.Fatalf("expected out-of-order error")
	}
}
