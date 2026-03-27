package tmuxsync

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alnah/panefleet/internal/state"
)

const ListPanesFormat = "#{pane_id}\t#{pane_dead}\t#{pane_dead_status}"

type PaneSnapshot struct {
	PaneID     string
	Dead       bool
	DeadStatus int
}

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
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			return nil, fmt.Errorf("line %d: expected 3 columns, got %d", lineNo, len(parts))
		}
		deadInt, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid pane_dead: %w", lineNo, err)
		}
		deadStatus, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid pane_dead_status: %w", lineNo, err)
		}
		out = append(out, PaneSnapshot{
			PaneID:     parts[0],
			Dead:       deadInt == 1,
			DeadStatus: deadStatus,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

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
