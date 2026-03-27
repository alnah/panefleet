package tmuxsync

import (
	"strings"
	"testing"
	"time"
)

func TestParseListPanesOutputErrors(t *testing.T) {
	cases := []string{
		"%1\t0",          // missing column
		"%1\tx\t0",       // bad dead
		"%1\t2\t0",       // unexpected dead flag
		"%1\t0\tinvalid", // bad dead status
	}
	for _, raw := range cases {
		if _, err := ParseListPanesOutput(raw); err == nil {
			t.Fatalf("expected parse error for %q", raw)
		}
	}
}

func TestParseListPanesOutputSkipsBlankLines(t *testing.T) {
	raw := strings.Join([]string{"", "%1\t0\t0", "", "%2\t1\t3", ""}, "\n")
	out, err := ParseListPanesOutput(raw)
	if err != nil {
		t.Fatalf("ParseListPanesOutput: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(out))
	}
}

func TestEventsFromSnapshotDefaultSource(t *testing.T) {
	events := EventsFromSnapshot([]PaneSnapshot{
		{PaneID: "%1", Dead: false},
		{PaneID: "%2", Dead: true, DeadStatus: 9},
	}, time.Now().UTC(), "")

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Source != "adapter:tmux-snapshot" {
		t.Fatalf("default source mismatch: %s", events[0].Source)
	}
	if events[1].ExitCode == nil || *events[1].ExitCode != 9 {
		t.Fatalf("expected exit code 9, got %#v", events[1].ExitCode)
	}
}
