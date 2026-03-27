package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/alnah/panefleet/internal/state"
)

func TestAppendAndProjectValidationErrors(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	err := s.AppendAndProject(ctx, state.Event{
		ID:         "bad-1",
		PaneID:     "",
		Kind:       state.EventPaneStarted,
		OccurredAt: now,
	}, state.PaneState{})
	if err == nil {
		t.Fatalf("expected event validation error")
	}

	err = s.AppendAndProject(ctx, state.Event{
		ID:         "",
		PaneID:     "%1",
		Kind:       state.EventPaneStarted,
		OccurredAt: now,
	}, state.PaneState{
		PaneID:           "%1",
		Status:           state.StatusRun,
		StatusSource:     "test",
		ReasonCode:       "test",
		Version:          1,
		LastEventAt:      now,
		LastTransitionAt: now,
	})
	if err == nil {
		t.Fatalf("expected event_id validation error")
	}

	err = s.AppendAndProject(ctx, state.Event{
		ID:         "bad-2",
		PaneID:     "%1",
		Kind:       state.EventPaneStarted,
		OccurredAt: now,
	}, state.PaneState{
		PaneID:           "%1",
		Status:           state.Status("NOPE"),
		StatusSource:     "test",
		ReasonCode:       "test",
		Version:          1,
		LastEventAt:      now,
		LastTransitionAt: now,
	})
	if err == nil {
		t.Fatalf("expected projection status validation error")
	}

	err = s.AppendAndProject(ctx, state.Event{
		ID:         "bad-3",
		PaneID:     "%1",
		Kind:       state.EventPaneStarted,
		OccurredAt: now,
	}, state.PaneState{
		PaneID:           "%2",
		Status:           state.StatusRun,
		StatusSource:     "test",
		ReasonCode:       "test",
		Version:          1,
		LastEventAt:      now,
		LastTransitionAt: now,
	})
	if err == nil {
		t.Fatalf("expected projection pane mismatch error")
	}

	err = s.AppendAndProject(ctx, state.Event{
		ID:         "bad-4",
		PaneID:     "%1",
		Kind:       state.EventPaneStarted,
		OccurredAt: now,
	}, state.PaneState{
		PaneID:           "%1",
		Status:           state.StatusRun,
		StatusSource:     "test",
		ReasonCode:       "test",
		Version:          0,
		LastEventAt:      now,
		LastTransitionAt: now,
	})
	if err == nil {
		t.Fatalf("expected projection version validation error")
	}

	err = s.AppendAndProject(ctx, state.Event{
		ID:         "bad-5",
		PaneID:     "%1",
		Kind:       state.EventPaneStarted,
		OccurredAt: now,
	}, state.PaneState{
		PaneID:           "%1",
		Status:           state.StatusRun,
		StatusSource:     "test",
		ReasonCode:       "test",
		Version:          1,
		LastEventAt:      now.Add(time.Second),
		LastTransitionAt: now,
	})
	if err == nil {
		t.Fatalf("expected projection last_event_at validation error")
	}

	err = s.AppendAndProject(ctx, state.Event{
		ID:         "bad-6",
		PaneID:     "%1",
		Kind:       state.EventPaneStarted,
		OccurredAt: now,
	}, state.PaneState{
		PaneID:           "%1",
		Status:           state.StatusRun,
		StatusSource:     "test",
		ReasonCode:       "test",
		Version:          1,
		LastEventAt:      now,
		LastTransitionAt: now.Add(time.Second),
	})
	if err == nil {
		t.Fatalf("expected projection last_transition_at validation error")
	}
}

func TestScanPaneStateHandlesNoRowsAndInvalidData(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, ok, err := s.GetPaneState(ctx, "%missing"); err != nil || ok {
		t.Fatalf("expected no row without error, ok=%v err=%v", ok, err)
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO pane_state(pane_id, status, status_source, reason_code, version, last_event_at, last_transition_at, last_exit_code, manual_override)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"%bad", "NOPE", "source", "reason", 1, "2026-03-27T10:00:00Z", "2026-03-27T10:00:00Z", nil, nil,
	)
	if err != nil {
		t.Fatalf("insert invalid row: %v", err)
	}
	if _, _, err := s.GetPaneState(ctx, "%bad"); err == nil {
		t.Fatalf("expected parse status error")
	}

	_, err = s.db.ExecContext(ctx, `DELETE FROM pane_state WHERE pane_id = ?`, "%bad")
	if err != nil {
		t.Fatalf("cleanup invalid row: %v", err)
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO pane_state(pane_id, status, status_source, reason_code, version, last_event_at, last_transition_at, last_exit_code, manual_override)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"%badtime", "RUN", "source", "reason", 1, "bad-time", "2026-03-27T10:00:00Z", nil, nil,
	)
	if err != nil {
		t.Fatalf("insert bad time row: %v", err)
	}
	if _, _, err := s.GetPaneState(ctx, "%badtime"); err == nil {
		t.Fatalf("expected parse time error")
	}

	_, err = s.db.ExecContext(ctx, `DELETE FROM pane_state WHERE pane_id = ?`, "%badtime")
	if err != nil {
		t.Fatalf("cleanup bad time row: %v", err)
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO pane_state(pane_id, status, status_source, reason_code, version, last_event_at, last_transition_at, last_exit_code, manual_override)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"", "RUN", "source", "reason", 0, "2026-03-27T10:00:00Z", "2026-03-27T10:00:01Z", nil, nil,
	)
	if err != nil {
		t.Fatalf("insert invalid persisted row: %v", err)
	}
	if _, _, err := s.GetPaneState(ctx, ""); err == nil {
		t.Fatalf("expected persisted state validation error")
	}
}

func TestScanPaneStateInternalNoRows(t *testing.T) {
	st, ok, err := scanPaneState(rowNoRows{})
	if err != nil {
		t.Fatalf("scanPaneState no rows err: %v", err)
	}
	if ok || st.PaneID != "" {
		t.Fatalf("expected empty state on no rows, got ok=%v state=%+v", ok, st)
	}
}

func TestAppendAndProjectDuplicateEventIsIdempotentAndList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	ev := state.Event{
		ID:         "evt-dup-1",
		PaneID:     "%201",
		Kind:       state.EventPaneStarted,
		OccurredAt: now,
		Source:     "test",
	}
	st := state.PaneState{
		PaneID:           "%201",
		Status:           state.StatusRun,
		StatusSource:     "test",
		ReasonCode:       "reason",
		Version:          1,
		LastEventAt:      now,
		LastTransitionAt: now,
	}

	if err := s.AppendAndProject(ctx, ev, st); err != nil {
		t.Fatalf("append initial: %v", err)
	}
	if err := s.AppendAndProject(ctx, ev, st); err != nil {
		t.Fatalf("append duplicate should be idempotent: %v", err)
	}

	var eventCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM events WHERE event_id = ?`, ev.ID).Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("duplicate event count=%d, want 1", eventCount)
	}

	// Add another pane to exercise ListPaneStates ordering/scan path.
	now2 := now.Add(1 * time.Second)
	ev2 := state.Event{
		ID:         "evt-dup-2",
		PaneID:     "%199",
		Kind:       state.EventPaneWaiting,
		OccurredAt: now2,
		Source:     "test",
	}
	st2 := state.PaneState{
		PaneID:           "%199",
		Status:           state.StatusWait,
		StatusSource:     "test",
		ReasonCode:       "reason",
		Version:          1,
		LastEventAt:      now2,
		LastTransitionAt: now2,
	}
	if err := s.AppendAndProject(ctx, ev2, st2); err != nil {
		t.Fatalf("append second pane: %v", err)
	}

	list, err := s.ListPaneStates(ctx)
	if err != nil {
		t.Fatalf("list pane states: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list len=%d, want 2", len(list))
	}
	if list[0].PaneID != "%199" || list[1].PaneID != "%201" {
		t.Fatalf("expected ASC pane ordering, got %q then %q", list[0].PaneID, list[1].PaneID)
	}
}

func TestAppendAndProjectRejectsDuplicateEventIDWithDifferentPayload(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	firstEvent := state.Event{
		ID:         "evt-conflict-1",
		PaneID:     "%301",
		Kind:       state.EventPaneStarted,
		OccurredAt: now,
		Source:     "test",
	}
	firstState := state.PaneState{
		PaneID:           "%301",
		Status:           state.StatusRun,
		StatusSource:     "test",
		ReasonCode:       "pane.started",
		Version:          1,
		LastEventAt:      now,
		LastTransitionAt: now,
	}
	if err := s.AppendAndProject(ctx, firstEvent, firstState); err != nil {
		t.Fatalf("append first event: %v", err)
	}

	secondEvent := state.Event{
		ID:         "evt-conflict-1",
		PaneID:     "%302",
		Kind:       state.EventPaneWaiting,
		OccurredAt: now,
		Source:     "test",
	}
	secondState := state.PaneState{
		PaneID:           "%302",
		Status:           state.StatusWait,
		StatusSource:     "test",
		ReasonCode:       "pane.waiting",
		Version:          1,
		LastEventAt:      now,
		LastTransitionAt: now,
	}
	if err := s.AppendAndProject(ctx, secondEvent, secondState); err == nil {
		t.Fatalf("expected duplicate event_id conflict")
	}
}

type rowNoRows struct{}

func (rowNoRows) Scan(_ ...any) error {
	return sql.ErrNoRows
}
