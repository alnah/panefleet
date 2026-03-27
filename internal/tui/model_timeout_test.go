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

func (blockingReader) SetOverride(ctx context.Context, paneID string, target state.Status, source string) (state.PaneState, error) {
	<-ctx.Done()
	return state.PaneState{}, ctx.Err()
}

func (blockingReader) ClearOverride(ctx context.Context, paneID, source string) (state.PaneState, error) {
	<-ctx.Done()
	return state.PaneState{}, ctx.Err()
}

func (blockingReader) KillPane(ctx context.Context, paneID string) error {
	<-ctx.Done()
	return ctx.Err()
}

func (blockingReader) RespawnPane(ctx context.Context, paneID string) error {
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
