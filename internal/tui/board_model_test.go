package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alnah/panefleet/internal/board"
	"github.com/alnah/panefleet/internal/state"
)

type fakeBoardRuntime struct {
	rows           []board.Row
	rowsErr        error
	previews       map[string]board.Preview
	previewErr     error
	toggleCalls    int
	jumpCalls      int
	killCalls      int
	respawnCalls   int
	lastPaneID     string
	lastJumpWindow string
	updates        chan state.PaneState
}

func (f *fakeBoardRuntime) Rows(context.Context) ([]board.Row, error) {
	if f.rowsErr != nil {
		return nil, f.rowsErr
	}
	out := make([]board.Row, len(f.rows))
	copy(out, f.rows)
	return out, nil
}

func (f *fakeBoardRuntime) Preview(_ context.Context, paneID string) (board.Preview, error) {
	if f.previewErr != nil {
		return board.Preview{}, f.previewErr
	}
	if preview, ok := f.previews[paneID]; ok {
		return preview, nil
	}
	return board.Preview{PaneID: paneID, Body: "preview"}, nil
}

func (f *fakeBoardRuntime) ToggleStaleOverride(_ context.Context, paneID string) (state.PaneState, error) {
	f.toggleCalls++
	f.lastPaneID = paneID
	return state.PaneState{PaneID: paneID, Status: state.StatusStale}, nil
}

func (f *fakeBoardRuntime) JumpToRow(_ context.Context, row board.Row) error {
	f.jumpCalls++
	f.lastPaneID = row.PaneID
	f.lastJumpWindow = row.TargetWindow()
	return nil
}

func (f *fakeBoardRuntime) KillPane(_ context.Context, paneID string) error {
	f.killCalls++
	f.lastPaneID = paneID
	return nil
}

func (f *fakeBoardRuntime) RespawnPane(_ context.Context, paneID string) error {
	f.respawnCalls++
	f.lastPaneID = paneID
	return nil
}

func (f *fakeBoardRuntime) Subscribe() (<-chan state.PaneState, func()) {
	if f.updates == nil {
		f.updates = make(chan state.PaneState)
	}
	return f.updates, func() {}
}

func TestBoardModelStartupAndSelection(t *testing.T) {
	runtime := &fakeBoardRuntime{
		rows: []board.Row{
			{PaneID: "%1", Status: state.StatusRun, SessionName: "work", WindowIndex: "1", PaneIndex: "0", WindowName: "clean", Repo: "panefleet"},
			{PaneID: "%2", Status: state.StatusIdle, SessionName: "work", WindowIndex: "2", PaneIndex: "0", WindowName: "zsh", Repo: "panefleet"},
		},
		previews: map[string]board.Preview{
			"%1": {PaneID: "%1", Status: state.StatusRun, SessionName: "work", WindowIndex: "1", PaneIndex: "0", WindowName: "clean", Body: "hello"},
		},
	}
	m := NewBoard(runtime, time.Second)
	updated, cmd := m.Update(boardRowsMsg{rows: runtime.rows})
	model := updated.(BoardModel)
	if cmd == nil {
		t.Fatalf("initial rows should request preview")
	}
	if model.selectedPaneID != "%1" {
		t.Fatalf("selectedPaneID = %q, want %%1", model.selectedPaneID)
	}

	updated, _ = model.Update(cmd())
	model = updated.(BoardModel)
	if model.preview.PaneID != "%1" {
		t.Fatalf("preview pane = %q, want %%1", model.preview.PaneID)
	}
}

func TestBoardModelNavigationQueuesPreviewOnly(t *testing.T) {
	runtime := &fakeBoardRuntime{
		rows: []board.Row{
			{PaneID: "%1", Status: state.StatusRun, SessionName: "work", WindowIndex: "1", PaneIndex: "0"},
			{PaneID: "%2", Status: state.StatusIdle, SessionName: "work", WindowIndex: "2", PaneIndex: "0"},
		},
	}
	m := NewBoard(runtime, time.Second)
	m.rows = runtime.rows
	m.selectedPaneID = "%1"
	m.rowsFetching = false

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model := updated.(BoardModel)
	if model.selectedPaneID != "%2" {
		t.Fatalf("selectedPaneID = %q, want %%2", model.selectedPaneID)
	}
	if cmd == nil {
		t.Fatalf("navigation should request preview")
	}
	if model.rowsFetching {
		t.Fatalf("navigation should not start a rows fetch")
	}
}

func TestBoardModelCoalescesBackgroundRefresh(t *testing.T) {
	m := NewBoard(&fakeBoardRuntime{}, time.Second)
	m.rowsFetching = true
	m.selectedPaneID = "%1"

	updated, cmd := m.Update(boardTickMsg(time.Now()))
	model := updated.(BoardModel)
	if cmd == nil {
		t.Fatalf("tick should reschedule itself")
	}
	if model.queuedRows == nil || *model.queuedRows != priorityBackground {
		t.Fatalf("background refresh should be queued while fetch is in flight")
	}

	updated, _ = model.Update(boardStateUpdatedMsg{paneID: "%1"})
	model = updated.(BoardModel)
	if model.queuedRows == nil || *model.queuedRows != priorityUser {
		t.Fatalf("user refresh should replace queued background refresh")
	}
}

func TestBoardModelTickRefreshesSelectedPreview(t *testing.T) {
	runtime := &fakeBoardRuntime{
		rows: []board.Row{{PaneID: "%1", SessionName: "work", WindowIndex: "1", PaneIndex: "0"}},
		previews: map[string]board.Preview{
			"%1": {PaneID: "%1", Status: state.StatusRun, Body: "preview"},
		},
	}
	m := NewBoard(runtime, time.Second)
	m.rows = runtime.rows
	m.rowsLoaded = true
	m.selectedPaneID = "%1"
	m.rowsFetching = false

	updated, cmd := m.Update(boardTickMsg(time.Now()))
	model := updated.(BoardModel)
	if cmd == nil {
		t.Fatalf("tick should schedule refresh commands")
	}
	if !model.rowsFetching {
		t.Fatalf("tick should start a rows fetch")
	}

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("tick command should be a batch, got %T", msg)
	}
	if len(batch) < 2 {
		t.Fatalf("tick batch should contain timer plus refresh commands, got %d", len(batch))
	}
}

func TestBoardModelLatestWinsPreviewQueue(t *testing.T) {
	m := NewBoard(&fakeBoardRuntime{}, time.Second)
	m.previewFetching = true

	if cmd := m.requestPreviewCmd("%1"); cmd != nil {
		t.Fatalf("requestPreviewCmd should queue while preview is in flight")
	}
	if cmd := m.requestPreviewCmd("%2"); cmd != nil {
		t.Fatalf("requestPreviewCmd should keep queuing while preview is in flight")
	}
	if m.queuedPreviewID != "%2" {
		t.Fatalf("queuedPreviewID = %q, want %%2", m.queuedPreviewID)
	}

	updated, cmd := m.Update(boardPreviewMsg{paneID: "%1", preview: board.Preview{PaneID: "%1"}})
	model := updated.(BoardModel)
	if cmd == nil {
		t.Fatalf("completion with queued preview should start next preview")
	}
	if !model.previewFetching {
		t.Fatalf("next preview should be in flight")
	}
}

func TestBoardModelActions(t *testing.T) {
	runtime := &fakeBoardRuntime{
		rows: []board.Row{{PaneID: "%1", SessionName: "work", WindowIndex: "1", PaneIndex: "0"}},
	}
	m := NewBoard(runtime, time.Second)
	m.rows = runtime.rows
	m.selectedPaneID = "%1"

	for _, key := range []rune{'s', 'd', 'x'} {
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		m = updated.(BoardModel)
		if cmd == nil {
			t.Fatalf("key %q should trigger action command", string(key))
		}
		updated, _ = m.Update(cmd())
		m = updated.(BoardModel)
	}
	if runtime.toggleCalls != 1 || runtime.killCalls != 1 || runtime.respawnCalls != 1 {
		t.Fatalf("unexpected action call counts: toggle=%d kill=%d respawn=%d", runtime.toggleCalls, runtime.killCalls, runtime.respawnCalls)
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(BoardModel)
	if cmd == nil {
		t.Fatalf("enter should trigger jump command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(BoardModel)
	if runtime.jumpCalls != 1 || runtime.lastJumpWindow != "work:1" {
		t.Fatalf("jump call mismatch: calls=%d target=%q", runtime.jumpCalls, runtime.lastJumpWindow)
	}
}

func TestBoardModelViewUsesFullScreenState(t *testing.T) {
	runtime := &fakeBoardRuntime{
		rows: []board.Row{{PaneID: "%1", Status: state.StatusRun, Tool: "codex", SessionName: "work", WindowIndex: "1", PaneIndex: "0", WindowName: "clean", Repo: "panefleet"}},
		previews: map[string]board.Preview{
			"%1": {PaneID: "%1", Status: state.StatusRun, Tool: "codex", SessionName: "work", WindowIndex: "1", PaneIndex: "0", WindowName: "clean", Body: "hello"},
		},
	}
	m := NewBoard(runtime, time.Second)
	m.rows = runtime.rows
	m.selectedPaneID = "%1"
	m.preview = runtime.previews["%1"]
	m.width = 100
	m.height = 20
	m.rowsLoaded = true

	view := m.View()
	if !strings.Contains(view, "Panefleet Board") || !strings.Contains(view, "TOKENS") || !strings.Contains(view, "hello") {
		t.Fatalf("view missing expected content: %q", view)
	}
	if got := len(strings.Split(view, "\n")); got != 20 {
		t.Fatalf("view line count = %d, want 20", got)
	}
}

func TestBoardModelViewShowsLoadingBeforeFirstRows(t *testing.T) {
	m := NewBoard(&fakeBoardRuntime{}, time.Second)
	m.width = 100
	m.height = 12

	view := m.View()
	if !strings.Contains(view, "loading panes...") {
		t.Fatalf("view should show loading panes state: %q", view)
	}
	if !strings.Contains(view, "loading preview...") {
		t.Fatalf("view should show loading preview state: %q", view)
	}
}

func TestBoardModelErrorPaths(t *testing.T) {
	runtime := &fakeBoardRuntime{rowsErr: errors.New("boom")}
	m := NewBoard(runtime, time.Second)
	updated, _ := m.Update(m.fetchRowsCmd(priorityStartup)())
	model := updated.(BoardModel)
	if model.err == nil || !strings.Contains(model.err.Error(), "boom") {
		t.Fatalf("expected fetch error, got %v", model.err)
	}
}
