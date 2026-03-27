package tmuxsync

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alnah/panefleet/internal/state"
)

// ListPanesFormat is the tmux format string expected by ParseListPanesOutput.
const ListPanesFormat = "#{pane_id}\t#{pane_dead}\t#{pane_dead_status}"

// PaneSnapshot represents one row from tmux list-panes output.
type PaneSnapshot struct {
	PaneID     string
	Dead       bool
	DeadStatus int
}

// ParseListPanesOutput parses tmux list-panes rows into strongly typed
// snapshots so adapter logic stays deterministic.
func ParseListPanesOutput(raw string) ([]PaneSnapshot, error) {
	out := make([]PaneSnapshot, 0)
	sc := bufio.NewScanner(strings.NewReader(raw))
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		item, err := parsePaneSnapshotLine(line, lineNo)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// EventsFromSnapshot converts tmux observations into reducer events so tmux
// remains an adapter source, not business logic.
func EventsFromSnapshot(items []PaneSnapshot, at time.Time, source string) []state.Event {
	if source == "" {
		source = "adapter:tmux-snapshot"
	}
	events := make([]state.Event, 0, len(items))
	for _, it := range items {
		ev := state.Event{
			PaneID:     it.PaneID,
			OccurredAt: at,
			Source:     source,
		}
		if it.Dead {
			code := it.DeadStatus
			ev.Kind = state.EventPaneExited
			ev.ExitCode = &code
			ev.ReasonCode = "tmux.pane_dead"
		} else {
			ev.Kind = state.EventPaneObserved
			ev.ReasonCode = "tmux.pane_observed"
		}
		events = append(events, ev)
	}
	return events
}

func parsePaneSnapshotLine(line string, lineNo int) (PaneSnapshot, error) {
	parts := strings.Split(line, "\t")
	if len(parts) != 3 {
		return PaneSnapshot{}, fmt.Errorf("line %d: expected 3 columns, got %d", lineNo, len(parts))
	}
	deadInt, err := strconv.Atoi(parts[1])
	if err != nil {
		return PaneSnapshot{}, fmt.Errorf("line %d: invalid pane_dead: %w", lineNo, err)
	}
	if deadInt != 0 && deadInt != 1 {
		return PaneSnapshot{}, fmt.Errorf("line %d: invalid pane_dead value %d", lineNo, deadInt)
	}
	deadStatus, err := strconv.Atoi(parts[2])
	if err != nil {
		return PaneSnapshot{}, fmt.Errorf("line %d: invalid pane_dead_status: %w", lineNo, err)
	}
	return PaneSnapshot{
		PaneID:     parts[0],
		Dead:       deadInt == 1,
		DeadStatus: deadStatus,
	}, nil
}
