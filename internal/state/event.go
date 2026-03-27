package state

import (
	"errors"
	"fmt"
	"time"
)

// EventKind describes state-changing inputs consumed by the reducer.
type EventKind string

const (
	EventPaneStarted     EventKind = "PaneStarted"
	EventPaneWaiting     EventKind = "PaneWaiting"
	EventPaneExited      EventKind = "PaneExited"
	EventPaneObserved    EventKind = "PaneObserved"
	EventOverrideSet     EventKind = "PaneOverrideSet"
	EventOverrideCleared EventKind = "PaneOverrideCleared"
	EventTimerRecompute  EventKind = "TimerRecompute"
)

// Event is a validated input for a single pane stream.
type Event struct {
	ID         string
	PaneID     string
	Kind       EventKind
	OccurredAt time.Time
	ExitCode   *int
	OverrideTo Status
	Source     string
	ReasonCode string
}

func (e Event) Validate() error {
	if e.PaneID == "" {
		return errors.New("pane_id is required")
	}
	if e.Kind == "" {
		return errors.New("event kind is required")
	}
	if e.OccurredAt.IsZero() {
		return errors.New("occurred_at is required")
	}

	switch e.Kind {
	case EventPaneStarted, EventPaneWaiting, EventPaneObserved, EventOverrideCleared, EventTimerRecompute:
		return nil
	case EventPaneExited:
		if e.ExitCode == nil {
			return errors.New("exit_code is required for pane exited")
		}
		return nil
	case EventOverrideSet:
		if !e.OverrideTo.Valid() {
			return fmt.Errorf("invalid override status: %q", e.OverrideTo)
		}
		return nil
	default:
		return fmt.Errorf("unsupported event kind: %q", e.Kind)
	}
}

// PaneState is the canonical state projection for one pane.
type PaneState struct {
	PaneID           string
	Status           Status
	StatusSource     string
	ReasonCode       string
	Version          uint64
	LastEventAt      time.Time
	LastTransitionAt time.Time
	LastExitCode     *int
	ManualOverride   *Status
}

func NewPaneState(paneID string) PaneState {
	now := time.Now().UTC()
	return PaneState{
		PaneID:           paneID,
		Status:           StatusUnknown,
		StatusSource:     "system:init",
		ReasonCode:       "init.unknown",
		Version:          0,
		LastEventAt:      now,
		LastTransitionAt: now,
	}
}
