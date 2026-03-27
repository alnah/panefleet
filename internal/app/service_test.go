package app

import (
	"context"
	"testing"
	"time"

	"github.com/alnah/panefleet/internal/state"
	"github.com/alnah/panefleet/internal/store"
)

func newService(t *testing.T) *Service {
	t.Helper()
	st, err := store.NewSQLiteStore("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("init store: %v", err)
	}

	reducer, err := state.NewReducer(state.Config{
		DoneRecentWindow: 10 * time.Minute,
		StaleWindow:      45 * time.Minute,
	})
	if err != nil {
		t.Fatalf("new reducer: %v", err)
	}
	return NewService(reducer, st)
}

func TestServiceIngestAndStateShow(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 26, 18, 0, 0, 0, time.UTC)

	_, err := svc.Ingest(ctx, state.Event{
		PaneID:     "%11",
		Kind:       state.EventPaneStarted,
		OccurredAt: at,
		Source:     "adapter:test",
	})
	if err != nil {
		t.Fatalf("ingest started: %v", err)
	}

	got, err := svc.StateShow(ctx, "%11")
	if err != nil {
		t.Fatalf("state-show: %v", err)
	}
	if got.Status != state.StatusRun {
		t.Fatalf("want RUN, got %s", got.Status)
	}
}

func TestServiceStateList(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 26, 18, 30, 0, 0, time.UTC)

	events := []state.Event{
		{
			PaneID:     "%21",
			Kind:       state.EventPaneStarted,
			OccurredAt: base,
		},
		{
			PaneID:     "%22",
			Kind:       state.EventPaneWaiting,
			OccurredAt: base.Add(1 * time.Second),
		},
	}
	for _, ev := range events {
		if _, err := svc.Ingest(ctx, ev); err != nil {
			t.Fatalf("ingest event: %v", err)
		}
	}

	all, err := svc.StateList(ctx)
	if err != nil {
		t.Fatalf("state-list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 states, got %d", len(all))
	}
}

func TestServiceSubscribePublishesOnIngest(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	ch, cancel := svc.Subscribe()
	defer cancel()

	at := time.Date(2026, 3, 26, 19, 0, 0, 0, time.UTC)
	if _, err := svc.Ingest(ctx, state.Event{
		PaneID:     "%31",
		Kind:       state.EventPaneStarted,
		OccurredAt: at,
	}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	select {
	case got := <-ch:
		if got.PaneID != "%31" {
			t.Fatalf("unexpected pane id: %s", got.PaneID)
		}
		if got.Status != state.StatusRun {
			t.Fatalf("unexpected status: %s", got.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for publish")
	}
}
