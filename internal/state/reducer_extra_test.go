package state

import (
	"testing"
	"time"
)

func TestReducerApplyTimerAndMismatchBranches(t *testing.T) {
	r := mustReducer(t)
	base := time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)

	_, err := r.Apply(PaneState{}, Event{
		PaneID:     "%1",
		Kind:       EventPaneStarted,
		OccurredAt: base,
	})
	if err != nil {
		t.Fatalf("apply start on empty state: %v", err)
	}

	_, err = r.Apply(NewPaneState("%1"), Event{
		PaneID:     "%2",
		Kind:       EventPaneObserved,
		OccurredAt: base,
	})
	if err == nil {
		t.Fatalf("expected pane mismatch error")
	}
}

func TestReducerApplyEmptyStateDoesNotDependOnWallClock(t *testing.T) {
	r := mustReducer(t)
	base := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)

	next, err := r.Apply(PaneState{}, Event{
		PaneID:     "%clock",
		Kind:       EventPaneStarted,
		OccurredAt: base,
	})
	if err != nil {
		t.Fatalf("Apply on empty state should accept first event regardless of wall clock: %v", err)
	}
	if next.LastEventAt != base {
		t.Fatalf("LastEventAt = %s, want %s", next.LastEventAt, base)
	}
	if next.LastTransitionAt != base {
		t.Fatalf("LastTransitionAt = %s, want %s", next.LastTransitionAt, base)
	}
}

func TestReducerApplyTimersAndHelpers(t *testing.T) {
	r := mustReducer(t)
	base := time.Date(2026, 3, 27, 13, 0, 0, 0, time.UTC)
	st := NewPaneState("%9")
	st.Status = Status("BROKEN")
	st.LastTransitionAt = base

	next, err := r.applyTimers(st, base.Add(time.Second), "src")
	if err != nil {
		t.Fatalf("applyTimers invalid status: %v", err)
	}
	if next.Status != StatusUnknown {
		t.Fatalf("invalid status should coerce to UNKNOWN, got %s", next.Status)
	}

	st = NewPaneState("%9")
	st.Status = StatusDone
	st.LastTransitionAt = base
	if _, err := r.applyTimers(st, base.Add(-time.Second), "src"); err == nil {
		t.Fatalf("expected negative duration error")
	}

	if defaultReason(Event{Kind: EventKind("NOPE")}) != "event.unknown" {
		t.Fatalf("defaultReason unknown mismatch")
	}
}

func TestReducerCopiesExitCodeValue(t *testing.T) {
	r := mustReducer(t)
	base := time.Date(2026, 3, 27, 14, 0, 0, 0, time.UTC)
	st := NewPaneState("%10")
	st.LastEventAt = base
	st.LastTransitionAt = base

	code := 7
	next, err := r.Apply(st, Event{
		PaneID:     "%10",
		Kind:       EventPaneExited,
		OccurredAt: base.Add(time.Second),
		ExitCode:   &code,
	})
	if err != nil {
		t.Fatalf("Apply exit: %v", err)
	}
	code = 0
	if next.LastExitCode == nil || *next.LastExitCode != 7 {
		t.Fatalf("exit code should be copied defensively, got %+v", next.LastExitCode)
	}
}
