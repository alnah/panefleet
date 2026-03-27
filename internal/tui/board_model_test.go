package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/alnah/panefleet/internal/board"
	"github.com/alnah/panefleet/internal/state"
)

type fakeBoardRuntime struct {
	rows           []board.Row
	rowsErr        error
	rowsFn         func(context.Context) ([]board.Row, error)
	previews       map[string]board.Preview
	previewErr     error
	previewFn      func(context.Context, string) (board.Preview, error)
	toggleCalls    int
	jumpCalls      int
	killCalls      int
	respawnCalls   int
	lastPaneID     string
	lastJumpWindow string
	updates        chan state.PaneState
}

func (f *fakeBoardRuntime) Rows(ctx context.Context) ([]board.Row, error) {
	if f.rowsFn != nil {
		return f.rowsFn(ctx)
	}
	if f.rowsErr != nil {
		return nil, f.rowsErr
	}
	out := make([]board.Row, len(f.rows))
	copy(out, f.rows)
	return out, nil
}

func (f *fakeBoardRuntime) Preview(ctx context.Context, paneID string) (board.Preview, error) {
	if f.previewFn != nil {
		return f.previewFn(ctx, paneID)
	}
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
	m := NewBoard(runtime, time.Second, "dracula")
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
	m := NewBoard(runtime, time.Second, "dracula")
	m.rows = runtime.rows
	m.selectedPaneID = "%1"
	m.rowsFetching = false

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
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
	m := NewBoard(&fakeBoardRuntime{}, time.Second, "dracula")
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
	m := NewBoard(runtime, time.Second, "dracula")
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
	m := NewBoard(&fakeBoardRuntime{}, time.Second, "dracula")
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
	m := NewBoard(runtime, time.Second, "dracula")
	m.rows = runtime.rows
	m.selectedPaneID = "%1"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(BoardModel)
	if cmd == nil {
		t.Fatalf("ctrl+s should trigger stale toggle command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(BoardModel)
	if runtime.toggleCalls != 1 {
		t.Fatalf("unexpected toggle call count: %d", runtime.toggleCalls)
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

func TestBoardModelEnterQuitsAfterSuccessfulJump(t *testing.T) {
	runtime := &fakeBoardRuntime{
		rows: []board.Row{{PaneID: "%1", SessionName: "work", WindowIndex: "1", PaneIndex: "0"}},
	}
	m := NewBoard(runtime, time.Second, "dracula")
	m.rows = runtime.rows
	m.selectedPaneID = "%1"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(BoardModel)
	if cmd == nil {
		t.Fatalf("enter should trigger jump command")
	}

	updated, quitCmd := m.Update(cmd())
	_ = updated.(BoardModel)
	if runtime.jumpCalls != 1 {
		t.Fatalf("jumpCalls = %d, want 1", runtime.jumpCalls)
	}
	if quitCmd == nil {
		t.Fatalf("successful jump should quit the board")
	}
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatalf("successful jump should return tea.Quit, got %T", quitCmd())
	}
}

func TestBoardModelViewUsesFullScreenState(t *testing.T) {
	runtime := &fakeBoardRuntime{
		rows: []board.Row{{PaneID: "%1", Status: state.StatusRun, Tool: "codex", SessionName: "work", WindowIndex: "1", PaneIndex: "0", WindowName: "clean", Repo: "panefleet"}},
		previews: map[string]board.Preview{
			"%1": {PaneID: "%1", Status: state.StatusRun, Tool: "codex", SessionName: "work", WindowIndex: "1", PaneIndex: "0", WindowName: "clean", Body: "hello"},
		},
	}
	m := NewBoard(runtime, time.Second, "dracula")
	m.rows = runtime.rows
	m.selectedPaneID = "%1"
	m.preview = runtime.previews["%1"]
	m.width = 100
	m.height = 20
	m.rowsLoaded = true

	view := m.View()
	if !strings.Contains(view, "BOARD") || !strings.Contains(view, "TOKENS") || !strings.Contains(view, "hello") {
		t.Fatalf("view missing expected content: %q", view)
	}
	if !strings.Contains(view, "┃ RUN") {
		t.Fatalf("view should highlight the selected row, got %q", view)
	}
	if got := len(strings.Split(view, "\n")); got != 20 {
		t.Fatalf("view line count = %d, want 20", got)
	}
}

func TestRenderTableRowSelectedFillsViewportWidth(t *testing.T) {
	m := NewBoard(&fakeBoardRuntime{}, time.Second, "dracula")
	row := board.Row{
		PaneID:      "%1",
		Status:      state.StatusRun,
		Tool:        "codex",
		SessionName: "work",
		WindowIndex: "1",
		PaneIndex:   "0",
		WindowName:  "clean",
		Repo:        "panefleet",
	}

	rendered := m.renderTableRow(row, true, false, 100)
	if got := ansi.StringWidth(rendered); got != 100 {
		t.Fatalf("selected row width = %d, want 100", got)
	}
}

func TestBoardModelViewShowsLoadingBeforeFirstRows(t *testing.T) {
	m := NewBoard(&fakeBoardRuntime{}, time.Second, "dracula")
	m.width = 100
	m.height = 12

	view := m.View()
	if !strings.Contains(view, "Panefleet >") {
		t.Fatalf("view should show search prompt: %q", view)
	}
	if !strings.Contains(view, "loading panes...") {
		t.Fatalf("view should show loading panes state: %q", view)
	}
	if !strings.Contains(view, "loading preview...") {
		t.Fatalf("view should show loading preview state: %q", view)
	}
}

func TestBoardModelErrorPaths(t *testing.T) {
	runtime := &fakeBoardRuntime{rowsErr: errors.New("boom")}
	m := NewBoard(runtime, time.Second, "dracula")
	updated, _ := m.Update(m.fetchRowsCmd(priorityStartup)())
	model := updated.(BoardModel)
	if model.err == nil || !strings.Contains(model.err.Error(), "boom") {
		t.Fatalf("expected fetch error, got %v", model.err)
	}
}

func TestBoardModelKeepsLastGoodRowsOnRefreshError(t *testing.T) {
	runtime := &fakeBoardRuntime{
		rows: []board.Row{
			{PaneID: "%1", Status: state.StatusRun, SessionName: "work", WindowIndex: "1", PaneIndex: "0"},
			{PaneID: "%2", Status: state.StatusIdle, SessionName: "work", WindowIndex: "2", PaneIndex: "0"},
		},
	}
	m := NewBoard(runtime, time.Second, "dracula")
	m.rows = append([]board.Row(nil), runtime.rows...)
	m.rowsLoaded = true
	m.selectedPaneID = "%2"
	m.rowsFetching = true

	updated, _ := m.Update(boardRowsMsg{err: errors.New("tmux down"), priority: priorityBackground})
	model := updated.(BoardModel)
	if model.err == nil || !strings.Contains(model.err.Error(), "tmux down") {
		t.Fatalf("expected rows error, got %v", model.err)
	}
	if len(model.rows) != 2 {
		t.Fatalf("rows len = %d, want 2", len(model.rows))
	}
	if model.selectedPaneID != "%2" {
		t.Fatalf("selectedPaneID = %q, want %%2", model.selectedPaneID)
	}
}

func TestBoardModelKeepsLastGoodPreviewOnRefreshError(t *testing.T) {
	runtime := &fakeBoardRuntime{}
	m := NewBoard(runtime, time.Second, "dracula")
	m.preview = board.Preview{PaneID: "%1", Status: state.StatusRun, Body: "stable preview"}
	m.previewFetching = true
	m.selectedPaneID = "%1"

	updated, _ := m.Update(boardPreviewMsg{paneID: "%1", err: errors.New("capture failed")})
	model := updated.(BoardModel)
	if model.err == nil || !strings.Contains(model.err.Error(), "capture failed") {
		t.Fatalf("expected preview error, got %v", model.err)
	}
	if model.preview.Body != "stable preview" {
		t.Fatalf("preview body = %q, want stable preview", model.preview.Body)
	}
}

func TestBoardModelFetchRowsCmdHonorsTimeout(t *testing.T) {
	runtime := &fakeBoardRuntime{
		rowsFn: func(ctx context.Context) ([]board.Row, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	m := NewBoard(runtime, time.Second, "dracula")
	m.opTimeout = 10 * time.Millisecond

	msg := m.fetchRowsCmd(priorityStartup)()
	rowsMsg, ok := msg.(boardRowsMsg)
	if !ok {
		t.Fatalf("msg type = %T, want boardRowsMsg", msg)
	}
	if rowsMsg.err == nil || !errors.Is(rowsMsg.err, context.DeadlineExceeded) {
		t.Fatalf("rows err = %v, want deadline exceeded", rowsMsg.err)
	}
}

func TestBoardModelFetchPreviewCmdHonorsTimeout(t *testing.T) {
	runtime := &fakeBoardRuntime{
		previewFn: func(ctx context.Context, paneID string) (board.Preview, error) {
			<-ctx.Done()
			return board.Preview{PaneID: paneID}, ctx.Err()
		},
	}
	m := NewBoard(runtime, time.Second, "dracula")
	m.previewTimeout = 10 * time.Millisecond

	msg := m.fetchPreviewCmd("%1")()
	previewMsg, ok := msg.(boardPreviewMsg)
	if !ok {
		t.Fatalf("msg type = %T, want boardPreviewMsg", msg)
	}
	if previewMsg.err == nil || !errors.Is(previewMsg.err, context.DeadlineExceeded) {
		t.Fatalf("preview err = %v, want deadline exceeded", previewMsg.err)
	}
}

func TestBoardModelSearchFiltersRowsAndUpdatesSelection(t *testing.T) {
	runtime := &fakeBoardRuntime{
		rows: []board.Row{
			{PaneID: "%1", Status: state.StatusRun, Tool: "codex", SessionName: "panefleet", WindowIndex: "1", PaneIndex: "0", WindowName: "tui", Repo: "panefleet"},
			{PaneID: "%2", Status: state.StatusIdle, Tool: "shell", SessionName: "professeur", WindowIndex: "2", PaneIndex: "0", WindowName: "mike", Repo: "fle"},
			{PaneID: "%3", Status: state.StatusDone, Tool: "codex", SessionName: "tcf_ninja", WindowIndex: "3", PaneIndex: "0", WindowName: "search", Repo: "tcf.ninja"},
		},
	}
	m := NewBoard(runtime, time.Second, "dracula")
	m.rows = runtime.rows
	m.rowsLoaded = true
	m.selectedPaneID = "%1"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = updated.(BoardModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = updated.(BoardModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(BoardModel)

	if m.searchQuery != "mik" {
		t.Fatalf("searchQuery = %q, want mik", m.searchQuery)
	}
	if got := len(m.filteredRows()); got != 1 {
		t.Fatalf("filtered rows = %d, want 1", got)
	}
	if m.selectedPaneID != "%2" {
		t.Fatalf("selectedPaneID = %q, want %%2", m.selectedPaneID)
	}
}

func TestBoardModelViewRendersSearchPromptWithQuery(t *testing.T) {
	runtime := &fakeBoardRuntime{
		rows: []board.Row{
			{PaneID: "%1", Status: state.StatusRun, Tool: "codex", SessionName: "panefleet", WindowIndex: "1", PaneIndex: "0", WindowName: "tui", Repo: "panefleet"},
		},
		previews: map[string]board.Preview{
			"%1": {PaneID: "%1", Status: state.StatusRun, Tool: "codex", SessionName: "panefleet", WindowIndex: "1", PaneIndex: "0", WindowName: "tui", Body: "hello"},
		},
	}
	m := NewBoard(runtime, time.Second, "dracula")
	m.rows = runtime.rows
	m.preview = runtime.previews["%1"]
	m.rowsLoaded = true
	m.selectedPaneID = "%1"
	m.searchQuery = "tui"
	m.width = 90
	m.height = 14

	view := m.View()
	if !strings.Contains(view, "Panefleet > tui") {
		t.Fatalf("view should show populated search prompt: %q", view)
	}
}

func TestBoardModelAltBackspaceClearsWholeSearch(t *testing.T) {
	runtime := &fakeBoardRuntime{
		rows: []board.Row{
			{PaneID: "%1", Status: state.StatusRun, Tool: "codex", SessionName: "panefleet", WindowIndex: "1", PaneIndex: "0", WindowName: "tui", Repo: "panefleet"},
			{PaneID: "%2", Status: state.StatusIdle, Tool: "shell", SessionName: "professeur", WindowIndex: "2", PaneIndex: "0", WindowName: "mike", Repo: "fle"},
		},
	}
	m := NewBoard(runtime, time.Second, "dracula")
	m.rows = runtime.rows
	m.rowsLoaded = true
	m.selectedPaneID = "%1"
	m.searchQuery = "panefleet"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace, Alt: true})
	m = updated.(BoardModel)

	if m.searchQuery != "" {
		t.Fatalf("searchQuery = %q, want empty", m.searchQuery)
	}
}

func TestBoardModelViewKeepsSearchPromptWithLargePreview(t *testing.T) {
	body := strings.Repeat("line\n", 80)
	runtime := &fakeBoardRuntime{
		rows: []board.Row{
			{PaneID: "%1", Status: state.StatusRun, Tool: "codex", SessionName: "panefleet", WindowIndex: "1", PaneIndex: "0", WindowName: "tui", Repo: "panefleet"},
		},
		previews: map[string]board.Preview{
			"%1": {PaneID: "%1", Status: state.StatusRun, Tool: "codex", SessionName: "panefleet", WindowIndex: "1", PaneIndex: "0", WindowName: "tui", Body: body},
		},
	}
	m := NewBoard(runtime, time.Second, "dracula")
	m.rows = runtime.rows
	m.preview = runtime.previews["%1"]
	m.rowsLoaded = true
	m.selectedPaneID = "%1"
	m.searchQuery = "abc"
	m.width = 90
	m.height = 10

	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], "Panefleet > abc") {
		t.Fatalf("search prompt should stay on first line: %q", view)
	}
}

func TestBoardModelViewKeepsSearchPromptWithWrappedPreviewLine(t *testing.T) {
	body := strings.Repeat("wrapped-preview-segment ", 40)
	runtime := &fakeBoardRuntime{
		rows: []board.Row{
			{PaneID: "%1", Status: state.StatusRun, Tool: "codex", SessionName: "panefleet", WindowIndex: "1", PaneIndex: "0", WindowName: "tui", Repo: "panefleet"},
		},
		previews: map[string]board.Preview{
			"%1": {PaneID: "%1", Status: state.StatusRun, Tool: "codex", SessionName: "panefleet", WindowIndex: "1", PaneIndex: "0", WindowName: "tui", Body: body},
		},
	}
	m := NewBoard(runtime, time.Second, "dracula")
	m.rows = runtime.rows
	m.preview = runtime.previews["%1"]
	m.rowsLoaded = true
	m.selectedPaneID = "%1"
	m.searchQuery = "abc"
	m.width = 90
	m.height = 10

	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], "Panefleet > abc") {
		t.Fatalf("search prompt should stay on first line even with long preview line: %q", view)
	}
}

func TestBoardModelEscQuits(t *testing.T) {
	m := NewBoard(&fakeBoardRuntime{}, time.Second, "dracula")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_ = updated.(BoardModel)
	if cmd == nil {
		t.Fatalf("esc should return quit command")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("esc should quit, got %T", msg)
	}
}

func TestBoardModelCtrlSUpdatesSelectedRowImmediately(t *testing.T) {
	runtime := &fakeBoardRuntime{
		rows: []board.Row{
			{PaneID: "%1", Status: state.StatusIdle, SessionName: "work", WindowIndex: "1", PaneIndex: "0"},
			{PaneID: "%2", Status: state.StatusIdle, SessionName: "work", WindowIndex: "2", PaneIndex: "0"},
		},
	}
	m := NewBoard(runtime, time.Second, "dracula")
	m.rows = append([]board.Row(nil), runtime.rows...)
	m.rowsLoaded = true
	m.selectedPaneID = "%1"
	m.rowsFetching = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(BoardModel)
	if cmd == nil {
		t.Fatalf("ctrl+s should trigger stale toggle command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(BoardModel)
	if m.rows[0].Status != state.StatusStale {
		t.Fatalf("row 0 status = %s, want STALE", m.rows[0].Status)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(BoardModel)
	if m.selectedPaneID != "%2" {
		t.Fatalf("selectedPaneID = %q, want %%2", m.selectedPaneID)
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(BoardModel)
	if cmd == nil {
		t.Fatalf("second ctrl+s should trigger stale toggle command immediately")
	}
	updated, _ = m.Update(cmd())
	m = updated.(BoardModel)
	if m.rows[1].Status != state.StatusStale {
		t.Fatalf("row 1 status = %s, want STALE", m.rows[1].Status)
	}
}
