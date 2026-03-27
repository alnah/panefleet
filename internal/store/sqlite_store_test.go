package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/alnah/panefleet/internal/state"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = s.Close()
	})
	if err := s.Init(context.Background()); err != nil {
		t.Fatalf("init sqlite store: %v", err)
	}
	return s
}

func TestSQLiteStoreAppendAndGetPaneState(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 26, 15, 0, 0, 0, time.UTC)

	zero := 0
	ev := state.Event{
		ID:         "ev-1",
		PaneID:     "%42",
		Kind:       state.EventPaneExited,
		OccurredAt: now,
		ExitCode:   &zero,
		Source:     "adapter:tmux",
		ReasonCode: "pane.exited",
	}
	override := state.StatusStale
	proj := state.PaneState{
		PaneID:           "%42",
		Status:           state.StatusDone,
		StatusSource:     "adapter:tmux",
		ReasonCode:       "pane.exited",
		Version:          2,
		LastEventAt:      now,
		LastTransitionAt: now,
		LastExitCode:     &zero,
		ManualOverride:   &override,
	}

	if err := s.AppendAndProject(ctx, ev, proj); err != nil {
		t.Fatalf("append and project: %v", err)
	}

	got, ok, err := s.GetPaneState(ctx, "%42")
	if err != nil {
		t.Fatalf("get pane state: %v", err)
	}
	if !ok {
		t.Fatalf("expected pane state")
	}
	if got.Status != state.StatusDone {
		t.Fatalf("status mismatch: got=%s", got.Status)
	}
	if got.Version != 2 {
		t.Fatalf("version mismatch: got=%d", got.Version)
	}
	if got.LastExitCode == nil || *got.LastExitCode != 0 {
		t.Fatalf("exit code mismatch: %+v", got.LastExitCode)
	}
	if got.ManualOverride == nil || *got.ManualOverride != state.StatusStale {
		t.Fatalf("manual override mismatch: %+v", got.ManualOverride)
	}
}

func TestSQLiteStoreIdempotentOnDuplicateEventID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 26, 16, 0, 0, 0, time.UTC)

	zero := 0
	ev := state.Event{
		ID:         "same-id",
		PaneID:     "%7",
		Kind:       state.EventPaneExited,
		OccurredAt: now,
		ExitCode:   &zero,
	}

	first := state.PaneState{
		PaneID:           "%7",
		Status:           state.StatusDone,
		StatusSource:     "adapter:tmux",
		ReasonCode:       "pane.exited",
		Version:          1,
		LastEventAt:      now,
		LastTransitionAt: now,
	}
	second := state.PaneState{
		PaneID:           "%7",
		Status:           state.StatusError,
		StatusSource:     "adapter:tmux",
		ReasonCode:       "pane.exited",
		Version:          99,
		LastEventAt:      now.Add(1 * time.Minute),
		LastTransitionAt: now.Add(1 * time.Minute),
	}

	if err := s.AppendAndProject(ctx, ev, first); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := s.AppendAndProject(ctx, ev, second); err != nil {
		t.Fatalf("second append (duplicate): %v", err)
	}

	got, ok, err := s.GetPaneState(ctx, "%7")
	if err != nil {
		t.Fatalf("get pane state: %v", err)
	}
	if !ok {
		t.Fatalf("expected state for pane")
	}
	if got.Version != 1 {
		t.Fatalf("expected unchanged version=1, got=%d", got.Version)
	}
	if got.Status != state.StatusDone {
		t.Fatalf("expected unchanged status DONE, got=%s", got.Status)
	}
}

func TestSQLiteStoreListPaneStates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 26, 17, 0, 0, 0, time.UTC)

	cases := []struct {
		id     string
		paneID string
		status state.Status
	}{
		{id: "a1", paneID: "%2", status: state.StatusRun},
		{id: "a2", paneID: "%1", status: state.StatusIdle},
	}

	for i, tc := range cases {
		ev := state.Event{
			ID:         tc.id,
			PaneID:     tc.paneID,
			Kind:       state.EventPaneObserved,
			OccurredAt: base.Add(time.Duration(i) * time.Second),
		}
		ps := state.PaneState{
			PaneID:           tc.paneID,
			Status:           tc.status,
			StatusSource:     "adapter:test",
			ReasonCode:       "test.case",
			Version:          uint64(i + 1),
			LastEventAt:      ev.OccurredAt,
			LastTransitionAt: ev.OccurredAt,
		}
		if err := s.AppendAndProject(ctx, ev, ps); err != nil {
			t.Fatalf("append %s: %v", tc.paneID, err)
		}
	}

	all, err := s.ListPaneStates(ctx)
	if err != nil {
		t.Fatalf("list pane states: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 states, got=%d", len(all))
	}
	if all[0].PaneID != "%1" || all[1].PaneID != "%2" {
		t.Fatalf("expected sorted pane ids, got=%s,%s", all[0].PaneID, all[1].PaneID)
	}
}

func TestSQLiteStoreInitConfiguresBusyTimeoutAndWAL(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "panefleet.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = s.Close()
	})
	if err := s.Init(context.Background()); err != nil {
		t.Fatalf("init sqlite store: %v", err)
	}

	var busyTimeout int
	if err := s.db.QueryRowContext(context.Background(), `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout=%d, want 5000", busyTimeout)
	}

	var journalMode string
	if err := s.db.QueryRowContext(context.Background(), `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode=%q, want wal", journalMode)
	}
}
