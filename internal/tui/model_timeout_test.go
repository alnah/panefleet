package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alnah/panefleet/internal/state"
)

type blockingReader struct{}

func (blockingReader) StateList(ctx context.Context) ([]state.PaneState, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (blockingReader) SetOverride(ctx context.Context, _ string, _ state.Status, _ string) (state.PaneState, error) {
	<-ctx.Done()
	return state.PaneState{}, ctx.Err()
}

func (blockingReader) ClearOverride(ctx context.Context, _, _ string) (state.PaneState, error) {
	<-ctx.Done()
	return state.PaneState{}, ctx.Err()
}

func (blockingReader) KillPane(ctx context.Context, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}

func (blockingReader) RespawnPane(ctx context.Context, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestModelFetchCmdUsesTimeoutContext(t *testing.T) {
	m := New(blockingReader{}, time.Millisecond, nil)
	m.opTimeout = 20 * time.Millisecond

	msg := m.fetchCmd()()
	got, ok := msg.(statesMsg)
	if !ok {
		t.Fatalf("expected statesMsg, got %T", msg)
	}
	if !errors.Is(got.err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", got.err)
	}
}

func TestModelActionCmdUsesTimeoutContext(t *testing.T) {
	m := New(blockingReader{}, time.Millisecond, nil)
	m.opTimeout = 20 * time.Millisecond

	msg := m.killCmd("%1")()
	got, ok := msg.(actionMsg)
	if !ok {
		t.Fatalf("expected actionMsg, got %T", msg)
	}
	if !errors.Is(got.err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", got.err)
	}
}

func TestModelSkipsOverlappingRefreshAndActions(t *testing.T) {
	m := New(blockingReader{}, time.Millisecond, nil)
	m.fetching = true
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("refresh should be skipped while a fetch is already in flight")
	}
	if !m.fetching {
		t.Fatalf("fetching flag should remain set")
	}
	if !m.refreshQueued {
		t.Fatalf("manual refresh should queue a follow-up fetch")
	}

	m.states = []state.PaneState{{PaneID: "%1", Status: state.StatusRun}}
	m.acting = true
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("action should be skipped while another action is already in flight")
	}
	if !m.acting {
		t.Fatalf("acting flag should remain set")
	}
}

func TestModelQueuesRefreshWhenActionFinishesDuringFetch(t *testing.T) {
	m := New(&fakeReader{}, time.Millisecond, nil)
	m.fetching = true
	m.acting = true

	updated, cmd := m.Update(actionMsg{err: nil})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("action completion should not start a second fetch immediately while one is in flight")
	}
	if !m.refreshQueued {
		t.Fatalf("action completion should queue a refresh")
	}

	updated, cmd = m.Update(statesMsg{states: []state.PaneState{{PaneID: "%1", Status: state.StatusRun}}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("queued refresh should trigger once the in-flight fetch finishes")
	}
	if !m.fetching {
		t.Fatalf("model should mark fetch as in-flight again")
	}
	if m.refreshQueued {
		t.Fatalf("queued refresh should be consumed")
	}
}
