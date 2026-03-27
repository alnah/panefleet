package tmuxsync

import (
	"testing"
	"time"

	"github.com/alnah/panefleet/internal/state"
)

func TestParseListPanesOutput(t *testing.T) {
	raw := "%1\t0\t0\n%2\t1\t2\n"
	got, err := ParseListPanesOutput(raw)
	if err != nil {
		t.Fatalf("parse list-panes output: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got))
	}
	if got[0].PaneID != "%1" || got[0].Dead {
		t.Fatalf("unexpected first row: %+v", got[0])
	}
	if got[1].PaneID != "%2" || !got[1].Dead || got[1].DeadStatus != 2 {
		t.Fatalf("unexpected second row: %+v", got[1])
	}
}

func TestEventsFromSnapshot(t *testing.T) {
	now := time.Date(2026, 3, 26, 20, 0, 0, 0, time.UTC)
	items := []PaneSnapshot{
		{PaneID: "%1", Dead: false, DeadStatus: 0},
		{PaneID: "%2", Dead: true, DeadStatus: 1},
	}
	out := EventsFromSnapshot(items, now, "adapter:tmux-snapshot")
	if len(out) != 2 {
		t.Fatalf("expected 2 events, got %d", len(out))
	}
	if out[0].Kind != state.EventPaneObserved {
		t.Fatalf("expected observed event, got %s", out[0].Kind)
	}
	if out[1].Kind != state.EventPaneExited || out[1].ExitCode == nil || *out[1].ExitCode != 1 {
		t.Fatalf("expected exited event with code=1, got %+v", out[1])
	}
}
