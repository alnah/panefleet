package state

import (
	"errors"
	"fmt"
	"time"
)

// Config contains lifecycle timing thresholds used by timer-driven transitions.
type Config struct {
	DoneRecentWindow time.Duration
	StaleWindow      time.Duration
}

// Validate guarantees timer windows are usable before state transitions run.
func (c Config) Validate() error {
	if c.DoneRecentWindow <= 0 {
		return errors.New("done recent window must be > 0")
	}
	if c.StaleWindow <= 0 {
		return errors.New("stale window must be > 0")
	}
	return nil
}

// Reducer is the canonical transition engine for pane state streams.
type Reducer struct {
	cfg Config
}

// NewReducer wires validated lifecycle timing rules into one reducer instance.
func NewReducer(cfg Config) (*Reducer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Reducer{cfg: cfg}, nil
}

// Apply projects one event onto current pane state while preserving ordering
// and manual override precedence.
func (r *Reducer) Apply(current PaneState, ev Event) (PaneState, error) {
	if err := ev.Validate(); err != nil {
		return current, err
	}
	if current.PaneID == "" {
		current = NewPaneState(ev.PaneID)
		current.LastEventAt = ev.OccurredAt
		current.LastTransitionAt = ev.OccurredAt
	}
	if current.PaneID != ev.PaneID {
		return current, fmt.Errorf("pane mismatch: state=%s event=%s", current.PaneID, ev.PaneID)
	}
	if !current.LastEventAt.IsZero() && ev.OccurredAt.Before(current.LastEventAt) {
		return current, fmt.Errorf("event out of order: event=%s last=%s", ev.OccurredAt, current.LastEventAt)
	}

	next := current
	next.Version++
	next.LastEventAt = ev.OccurredAt

	source := ev.Source
	if source == "" {
		source = "adapter:unknown"
	}
	reason := ev.ReasonCode
	if reason == "" {
		reason = defaultReason(ev)
	}

	switch ev.Kind {
	case EventOverrideSet:
		o := ev.OverrideTo
		next.ManualOverride = &o
		return next, nil
	case EventOverrideCleared:
		next.ManualOverride = nil
		return r.applyTimers(next, ev.OccurredAt, source)
	case EventTimerRecompute:
		return r.applyTimers(next, ev.OccurredAt, source)
	}

	switch ev.Kind {
	case EventPaneStarted:
		next = r.setStatus(next, StatusRun, source, reason, ev.OccurredAt)
	case EventPaneWaiting:
		next = r.setStatus(next, StatusWait, source, reason, ev.OccurredAt)
	case EventPaneObserved:
		next, _ = r.applyTimers(next, ev.OccurredAt, source)
	case EventPaneExited:
		exitCode := *ev.ExitCode
		next.LastExitCode = &exitCode
		if *ev.ExitCode == 0 {
			next = r.setStatus(next, StatusDone, source, reason, ev.OccurredAt)
		} else {
			next = r.setStatus(next, StatusError, source, reason, ev.OccurredAt)
		}
	default:
		return next, fmt.Errorf("unsupported event: %s", ev.Kind)
	}

	return next, nil
}

func (r *Reducer) applyTimers(state PaneState, now time.Time, source string) (PaneState, error) {
	if !state.Status.Valid() {
		state = r.setStatus(state, StatusUnknown, source, "status.invalid", now)
		return state, nil
	}

	delta := now.Sub(state.LastTransitionAt)
	if delta < 0 {
		return state, fmt.Errorf("negative duration: now=%s transition=%s", now, state.LastTransitionAt)
	}

	switch state.Status {
	case StatusDone:
		if delta >= r.cfg.DoneRecentWindow {
			state = r.setStatus(state, StatusIdle, source, "timer.done_to_idle", now)
		}
	case StatusIdle:
		if delta >= r.cfg.StaleWindow {
			state = r.setStatus(state, StatusStale, source, "timer.idle_to_stale", now)
		}
	}
	return state, nil
}

func (r *Reducer) setStatus(state PaneState, s Status, source, reason string, ts time.Time) PaneState {
	if !s.Valid() {
		s = StatusUnknown
	}
	if state.Status != s {
		state.LastTransitionAt = ts
	}
	state.Status = s
	state.StatusSource = source
	state.ReasonCode = reason
	return state
}

func defaultReason(ev Event) string {
	switch ev.Kind {
	case EventPaneStarted:
		return "pane.started"
	case EventPaneWaiting:
		return "pane.waiting"
	case EventPaneExited:
		return "pane.exited"
	case EventPaneObserved:
		return "pane.observed"
	case EventOverrideSet:
		return "override.set"
	case EventOverrideCleared:
		return "override.cleared"
	case EventTimerRecompute:
		return "timer.recompute"
	default:
		return "event.unknown"
	}
}
