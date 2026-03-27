package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alnah/panefleet/internal/state"
)

func TestServiceOverrideMethods(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	at := time.Now().UTC().Add(-1 * time.Second)

	if _, err := svc.Ingest(ctx, state.Event{
		PaneID:     "%61",
		Kind:       state.EventPaneStarted,
		OccurredAt: at,
	}); err != nil {
		t.Fatalf("ingest start: %v", err)
	}

	if _, err := svc.SetOverride(ctx, "%61", state.StatusStale, "test"); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	out, err := svc.StateShow(ctx, "%61")
	if err != nil {
		t.Fatalf("StateShow: %v", err)
	}
	if out.Status != state.StatusStale {
		t.Fatalf("override status = %s, want STALE", out.Status)
	}

	if _, err := svc.ClearOverride(ctx, "%61", "test"); err != nil {
		t.Fatalf("ClearOverride: %v", err)
	}
}

func TestServiceClearOverrideRestoresUnderlyingState(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-2 * time.Second)

	if _, err := svc.Ingest(ctx, state.Event{
		PaneID:     "%62",
		Kind:       state.EventPaneStarted,
		OccurredAt: base,
		Source:     "adapter:test",
	}); err != nil {
		t.Fatalf("ingest start: %v", err)
	}
	if _, err := svc.SetOverride(ctx, "%62", state.StatusStale, "test"); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}

	exitCode := 2
	exitAt := time.Now().UTC()
	shown, err := svc.Ingest(ctx, state.Event{
		PaneID:     "%62",
		Kind:       state.EventPaneExited,
		OccurredAt: exitAt,
		ExitCode:   &exitCode,
		Source:     "adapter:test",
	})
	if err != nil {
		t.Fatalf("ingest exit under override: %v", err)
	}
	if shown.Status != state.StatusStale {
		t.Fatalf("override should still mask state, got %s", shown.Status)
	}

	if _, err := svc.ClearOverride(ctx, "%62", "test"); err != nil {
		t.Fatalf("ClearOverride: %v", err)
	}
	got, err := svc.StateShow(ctx, "%62")
	if err != nil {
		t.Fatalf("StateShow after clear: %v", err)
	}
	if got.Status != state.StatusError {
		t.Fatalf("clear override should reveal underlying ERROR, got %s", got.Status)
	}
	if got.LastExitCode == nil || *got.LastExitCode != exitCode {
		t.Fatalf("last exit code mismatch: %+v", got.LastExitCode)
	}
}

func TestServiceStateShowErrors(t *testing.T) {
	svc := newService(t)
	_, err := svc.StateShow(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "pane id is required") {
		t.Fatalf("expected pane id required, got: %v", err)
	}

	_, err = svc.StateShow(context.Background(), "%missing")
	if err == nil || !strings.Contains(err.Error(), "pane not found") {
		t.Fatalf("expected pane not found, got: %v", err)
	}
}

func TestServiceOverrideMethodsTolerateFutureLastEventAt(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	future := time.Now().UTC().Add(2 * time.Minute)

	if _, err := svc.Ingest(ctx, state.Event{
		PaneID:     "%63",
		Kind:       state.EventPaneStarted,
		OccurredAt: future,
		Source:     "adapter:test",
	}); err != nil {
		t.Fatalf("ingest future start: %v", err)
	}

	if _, err := svc.SetOverride(ctx, "%63", state.StatusStale, "test"); err != nil {
		t.Fatalf("SetOverride with future state: %v", err)
	}
	if _, err := svc.ClearOverride(ctx, "%63", "test"); err != nil {
		t.Fatalf("ClearOverride with future state: %v", err)
	}
}
