package state

import "fmt"

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

func (s Status) Valid() bool {
	switch s {
	case StatusUnknown, StatusIdle, StatusRun, StatusWait, StatusDone, StatusError, StatusStale:
		return true
	default:
		return false
	}
}

func ParseStatus(raw string) (Status, error) {
	s := Status(raw)
	if !s.Valid() {
		return "", fmt.Errorf("invalid status: %s", raw)
	}
	return s, nil
}
