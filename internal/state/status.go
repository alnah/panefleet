package state

import (
	"fmt"
	"strings"
)

// Status is the canonical pane state.
type Status string

const (
	StatusUnknown Status = "UNKNOWN"
	StatusIdle    Status = "IDLE"
	StatusRun     Status = "RUN"
	StatusWait    Status = "WAIT"
	StatusDone    Status = "DONE"
	StatusError   Status = "ERROR"
	StatusStale   Status = "STALE"
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
