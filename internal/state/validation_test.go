package state

import (
	"testing"
	"time"
)

func TestEventValidateBranches(t *testing.T) {
	now := time.Now().UTC()
	code := 0

	validCases := []Event{
		{PaneID: "%1", Kind: EventPaneStarted, OccurredAt: now},
		{PaneID: "%1", Kind: EventPaneWaiting, OccurredAt: now},
		{PaneID: "%1", Kind: EventPaneObserved, OccurredAt: now},
		{PaneID: "%1", Kind: EventOverrideCleared, OccurredAt: now},
		{PaneID: "%1", Kind: EventTimerRecompute, OccurredAt: now},
		{PaneID: "%1", Kind: EventPaneExited, OccurredAt: now, ExitCode: &code},
		{PaneID: "%1", Kind: EventOverrideSet, OccurredAt: now, OverrideTo: StatusStale},
	}
	for _, ev := range validCases {
		if err := ev.Validate(); err != nil {
			t.Fatalf("expected valid event %+v, got %v", ev, err)
		}
	}

	invalid := []Event{
		{Kind: EventPaneStarted, OccurredAt: now},
		{PaneID: "%1", OccurredAt: now},
		{PaneID: "%1", Kind: EventPaneStarted},
		{PaneID: "%1", Kind: EventPaneExited, OccurredAt: now},
		{PaneID: "%1", Kind: EventOverrideSet, OccurredAt: now, OverrideTo: Status("NOPE")},
		{PaneID: "%1", Kind: EventKind("NOPE"), OccurredAt: now},
	}
	for _, ev := range invalid {
		if err := ev.Validate(); err == nil {
			t.Fatalf("expected invalid event %+v", ev)
		}
	}
}

func TestReducerConfigValidation(t *testing.T) {
	if _, err := NewReducer(Config{DoneRecentWindow: 0, StaleWindow: time.Minute}); err == nil {
		t.Fatalf("expected done window validation error")
	}
	if _, err := NewReducer(Config{DoneRecentWindow: time.Minute, StaleWindow: 0}); err == nil {
		t.Fatalf("expected stale window validation error")
	}
}
