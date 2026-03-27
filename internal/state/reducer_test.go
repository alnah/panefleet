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
	if idleState.ReasonCode != "timer.done_to_idle" {
		t.Fatalf("want timer.done_to_idle, got %s", idleState.ReasonCode)
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
	if staleState.ReasonCode != "timer.idle_to_stale" {
		t.Fatalf("want timer.idle_to_stale, got %s", staleState.ReasonCode)
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
	if overrideState.ManualOverride == nil || *overrideState.ManualOverride != StatusStale {
		t.Fatalf("override should be recorded, got %+v", overrideState.ManualOverride)
	}
	if overrideState.Status != StatusUnknown {
		t.Fatalf("override should not replace underlying status, got %s", overrideState.Status)
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
	if afterStart.Status != StatusRun {
		t.Fatalf("underlying state should keep progressing, got %s", afterStart.Status)
	}
	if afterStart.ManualOverride == nil || *afterStart.ManualOverride != StatusStale {
		t.Fatalf("override should remain active, got %+v", afterStart.ManualOverride)
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
	if cleared.Status != StatusRun {
		t.Fatalf("cleared override should expose underlying status, got %s", cleared.Status)
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

func TestReducerClearOverrideKeepsTimerReason(t *testing.T) {
	r := mustReducer(t)
	base := time.Date(2026, 3, 27, 15, 0, 0, 0, time.UTC)
	override := StatusStale
	st := PaneState{
		PaneID:           "%5",
		Status:           StatusDone,
		StatusSource:     "adapter:tmux",
		ReasonCode:       "pane.exited",
		Version:          3,
		LastEventAt:      base,
		LastTransitionAt: base.Add(-11 * time.Minute),
		ManualOverride:   &override,
	}

	cleared, err := r.Apply(st, Event{
		PaneID:     "%5",
		Kind:       EventOverrideCleared,
		OccurredAt: base,
		Source:     "manual",
		ReasonCode: "override.cleared",
	})
	if err != nil {
		t.Fatalf("clear override: %v", err)
	}
	if cleared.Status != StatusIdle {
		t.Fatalf("want IDLE, got %s", cleared.Status)
	}
	if cleared.ReasonCode != "timer.done_to_idle" {
		t.Fatalf("want timer.done_to_idle, got %s", cleared.ReasonCode)
	}
}
