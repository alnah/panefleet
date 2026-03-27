package state

import (
	"fmt"
	"strings"
)

// Status is the canonical pane state.
type Status string

const (
	// StatusUnknown means no reliable lifecycle state has been observed yet.
	StatusUnknown Status = "UNKNOWN"
	// StatusIdle means the pane is alive but currently inactive.
	StatusIdle Status = "IDLE"
	// StatusRun means the pane is actively running work.
	StatusRun Status = "RUN"
	// StatusWait means the pane is waiting on external input or approval.
	StatusWait Status = "WAIT"
	// StatusDone means the pane exited successfully and is still recent.
	StatusDone Status = "DONE"
	// StatusError means the pane exited with a non-zero status.
	StatusError Status = "ERROR"
	// StatusStale means no recent activity has been observed for the pane.
	StatusStale Status = "STALE"
)

// Valid reports whether the status is one of the lifecycle values accepted by
// reducers and storage.
func (s Status) Valid() bool {
	switch s {
	case StatusUnknown, StatusIdle, StatusRun, StatusWait, StatusDone, StatusError, StatusStale:
		return true
	default:
		return false
	}
}

// ParseStatus converts persisted/user status text to canonical enum values so
// invalid statuses fail at boundaries instead of leaking into reducer logic.
func ParseStatus(raw string) (Status, error) {
	normalized := strings.TrimSpace(raw)
	s := Status(normalized)
	if !s.Valid() {
		return "", fmt.Errorf("invalid status: %q", raw)
	}
	return s, nil
}
