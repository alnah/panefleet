package board

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alnah/panefleet/internal/state"
	"github.com/alnah/panefleet/internal/tmuxctl"
)

type fakeStateSource struct {
	list       []state.PaneState
	show       map[string]state.PaneState
	listErr    error
	showErr    error
	setCalls   int
	clearCalls int
	lastPaneID string
	updates    chan state.PaneState
}

func (f *fakeStateSource) StateShow(_ context.Context, paneID string) (state.PaneState, error) {
	if f.showErr != nil {
		return state.PaneState{}, f.showErr
	}
	if st, ok := f.show[paneID]; ok {
		return st, nil
	}
	return state.PaneState{}, errors.New("not found")
}

func (f *fakeStateSource) StateList(context.Context) ([]state.PaneState, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]state.PaneState, len(f.list))
	copy(out, f.list)
	return out, nil
}

func (f *fakeStateSource) SetOverride(_ context.Context, paneID string, _ state.Status, _ string) (state.PaneState, error) {
	f.setCalls++
	f.lastPaneID = paneID
	return state.PaneState{PaneID: paneID, Status: state.StatusStale}, nil
}

func (f *fakeStateSource) ClearOverride(_ context.Context, paneID, _ string) (state.PaneState, error) {
	f.clearCalls++
	f.lastPaneID = paneID
	return state.PaneState{PaneID: paneID, Status: state.StatusIdle}, nil
}

func (f *fakeStateSource) Subscribe() (<-chan state.PaneState, func()) {
	if f.updates == nil {
		f.updates = make(chan state.PaneState)
	}
	return f.updates, func() {}
}

type fakeTMUX struct {
	snapshot    []tmuxctl.BoardPane
	snapshotErr error
	preview     tmuxctl.PanePreview
	previewErr  error
	captures    map[string]string
	jumpPane    string
	jumpWindow  string
	killPane    string
	respPane    string
}

func (f *fakeTMUX) BoardSnapshot(context.Context) ([]tmuxctl.BoardPane, error) {
	if f.snapshotErr != nil {
		return nil, f.snapshotErr
	}
	out := make([]tmuxctl.BoardPane, len(f.snapshot))
	copy(out, f.snapshot)
	return out, nil
}

func (f *fakeTMUX) Preview(context.Context, string) (tmuxctl.PanePreview, error) {
	if f.previewErr != nil {
		return tmuxctl.PanePreview{}, f.previewErr
	}
	return f.preview, nil
}

func (f *fakeTMUX) RecentCapture(_ context.Context, paneID string) (string, error) {
	if capture, ok := f.captures[paneID]; ok {
		return capture, nil
	}
	return "", nil
}

func (f *fakeTMUX) JumpToPane(_ context.Context, paneID, targetWindow string) error {
	f.jumpPane = paneID
	f.jumpWindow = targetWindow
	return nil
}

func (f *fakeTMUX) KillPane(_ context.Context, paneID string) error {
	f.killPane = paneID
	return nil
}

func (f *fakeTMUX) RespawnPane(_ context.Context, paneID string) error {
	f.respPane = paneID
	return nil
}

func TestRowsJoinAndSort(t *testing.T) {
	stale := state.StatusStale
	svc := NewService(
		&fakeStateSource{
			list: []state.PaneState{
				{PaneID: "%2", Status: state.StatusIdle},
				{PaneID: "%1", Status: state.StatusRun, ManualOverride: &stale},
			},
		},
		&fakeTMUX{
			snapshot: []tmuxctl.BoardPane{
				{
					PaneID:         "%2",
					SessionName:    "work",
					WindowIndex:    "2",
					WindowName:     "zsh",
					PaneIndex:      "0",
					Command:        "zsh",
					Title:          "shell",
					Path:           "/tmp/example",
					WindowActivity: time.Unix(100, 0).UTC(),
				},
				{
					PaneID:         "%1",
					SessionName:    "work",
					WindowIndex:    "1",
					WindowName:     "clean",
					PaneIndex:      "0",
					Command:        "codex-aarch64-a",
					Title:          "cdx",
					Path:           "/tmp/panefleet",
					TokensUsed:     intPtr(123),
					ContextLeftPct: intPtr(88),
					WindowActivity: time.Unix(200, 0).UTC(),
				},
			},
		},
		"",
	)

	rows, err := svc.Rows(context.Background())
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows len = %d, want 2", len(rows))
	}
	if rows[0].PaneID != "%1" {
		t.Fatalf("rows[0].PaneID = %q, want %%1", rows[0].PaneID)
	}
	if rows[0].Tool != "codex" {
		t.Fatalf("rows[0].Tool = %q, want codex", rows[0].Tool)
	}
	if rows[0].Repo != "panefleet" {
		t.Fatalf("rows[0].Repo = %q, want panefleet", rows[0].Repo)
	}
	if rows[0].TargetPane() != "1.0" {
		t.Fatalf("rows[0].TargetPane = %q, want 1.0", rows[0].TargetPane())
	}
	if rows[0].TargetWindow() != "work:1" {
		t.Fatalf("rows[0].TargetWindow = %q, want work:1", rows[0].TargetWindow())
	}
	if rows[0].TokensUsed == nil || *rows[0].TokensUsed != 123 {
		t.Fatalf("rows[0].TokensUsed = %v, want 123", rows[0].TokensUsed)
	}
	if rows[0].ContextLeftPct == nil || *rows[0].ContextLeftPct != 88 {
		t.Fatalf("rows[0].ContextLeftPct = %v, want 88", rows[0].ContextLeftPct)
	}
	if rows[0].ManualOverride == nil || *rows[0].ManualOverride != state.StatusStale {
		t.Fatalf("rows[0].ManualOverride = %v, want STALE", rows[0].ManualOverride)
	}
	if rows[1].Tool != "shell" {
		t.Fatalf("rows[1].Tool = %q, want shell", rows[1].Tool)
	}
}

func TestPreviewUsesLivePaneAndState(t *testing.T) {
	svc := NewService(
		&fakeStateSource{
			show: map[string]state.PaneState{
				"%1": {PaneID: "%1", Status: state.StatusDone},
			},
		},
		&fakeTMUX{
			preview: tmuxctl.PanePreview{
				PaneID:      "%1",
				SessionName: "work",
				WindowIndex: "1",
				WindowName:  "clean",
				PaneIndex:   "0",
				Command:     "codex-aarch64-a",
				Title:       "cdx",
				Path:        "/tmp/panefleet",
				Body:        "hello\nworld",
			},
		},
		"",
	)
	now := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	got, err := svc.Preview(context.Background(), "%1")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if got.Status != state.StatusDone {
		t.Fatalf("got.Status = %s, want DONE", got.Status)
	}
	if got.Tool != "codex" {
		t.Fatalf("got.Tool = %q, want codex", got.Tool)
	}
	if got.LoadedAt != now {
		t.Fatalf("got.LoadedAt = %v, want %v", got.LoadedAt, now)
	}
}

func TestPreviewFallsBackWhenPaneStateIsMissing(t *testing.T) {
	svc := NewService(
		&fakeStateSource{},
		&fakeTMUX{
			preview: tmuxctl.PanePreview{
				PaneID:         "%1",
				SessionName:    "work",
				WindowIndex:    "1",
				WindowName:     "clean",
				PaneIndex:      "0",
				Command:        "codex-aarch64-a",
				Title:          "cdx",
				Path:           "/tmp/panefleet/",
				AgentStatus:    "RUN",
				AgentTool:      "CODEX",
				AgentUpdatedAt: time.Now().UTC(),
				Body:           "hello\nworld",
			},
		},
		"",
	)

	got, err := svc.Preview(context.Background(), "%1")
	if err != nil {
		t.Fatalf("Preview fallback: %v", err)
	}
	if got.Status != state.StatusRun {
		t.Fatalf("got.Status = %s, want RUN", got.Status)
	}
}

func TestToggleStaleOverride(t *testing.T) {
	stale := state.StatusStale
	source := &fakeStateSource{
		show: map[string]state.PaneState{
			"%1": {PaneID: "%1", Status: state.StatusRun},
			"%2": {PaneID: "%2", Status: state.StatusStale, ManualOverride: &stale},
		},
	}
	svc := NewService(source, &fakeTMUX{}, "")

	if _, err := svc.ToggleStaleOverride(context.Background(), "%1"); err != nil {
		t.Fatalf("ToggleStaleOverride set: %v", err)
	}
	if source.setCalls != 1 || source.lastPaneID != "%1" {
		t.Fatalf("set override calls = %d pane=%q", source.setCalls, source.lastPaneID)
	}

	if _, err := svc.ToggleStaleOverride(context.Background(), "%2"); err != nil {
		t.Fatalf("ToggleStaleOverride clear: %v", err)
	}
	if source.clearCalls != 1 || source.lastPaneID != "%2" {
		t.Fatalf("clear override calls = %d pane=%q", source.clearCalls, source.lastPaneID)
	}
}

func TestJumpKillRespawnAndSubscribe(t *testing.T) {
	source := &fakeStateSource{}
	tmux := &fakeTMUX{}
	svc := NewService(source, tmux, "")

	row := Row{PaneID: "%1", SessionName: "work", WindowIndex: "3"}
	if err := svc.JumpToRow(context.Background(), row); err != nil {
		t.Fatalf("JumpToRow: %v", err)
	}
	if tmux.jumpPane != "%1" || tmux.jumpWindow != "work:3" {
		t.Fatalf("jump call = pane %q window %q", tmux.jumpPane, tmux.jumpWindow)
	}
	if err := svc.KillPane(context.Background(), "%2"); err != nil {
		t.Fatalf("KillPane: %v", err)
	}
	if err := svc.RespawnPane(context.Background(), "%3"); err != nil {
		t.Fatalf("RespawnPane: %v", err)
	}
	if tmux.killPane != "%2" || tmux.respPane != "%3" {
		t.Fatalf("tmux actions kill=%q respawn=%q", tmux.killPane, tmux.respPane)
	}

	ch, cancel := svc.Subscribe()
	if ch == nil {
		t.Fatalf("Subscribe returned nil channel")
	}
	cancel()
}

func TestRowsHideCurrentBoardPane(t *testing.T) {
	svc := NewService(
		&fakeStateSource{},
		&fakeTMUX{
			snapshot: []tmuxctl.BoardPane{
				{PaneID: "%1", SessionName: "work", WindowIndex: "1", PaneIndex: "0"},
				{PaneID: "%2", SessionName: "work", WindowIndex: "2", PaneIndex: "0"},
			},
		},
		"%1",
	)

	rows, err := svc.Rows(context.Background())
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if len(rows) != 1 || rows[0].PaneID != "%2" {
		t.Fatalf("rows = %+v, want only %%2", rows)
	}
}

func TestRowsUseCaptureHeuristicWhenNoBetterStatusExists(t *testing.T) {
	svc := NewService(
		&fakeStateSource{},
		&fakeTMUX{
			snapshot: []tmuxctl.BoardPane{
				{
					PaneID:         "%1",
					SessionName:    "work",
					WindowIndex:    "1",
					WindowName:     "codex",
					PaneIndex:      "0",
					Command:        "codex-aarch64-a",
					Title:          "cdx",
					WindowActivity: time.Now().UTC(),
				},
			},
			captures: map[string]string{
				"%1": "OpenAI Codex\nwaiting on approval",
			},
		},
		"",
	)

	rows, err := svc.Rows(context.Background())
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].Status != state.StatusWait {
		t.Fatalf("rows[0].Status = %s, want WAIT", rows[0].Status)
	}
}

func TestRowsUseCodexDoneAndClaudeErrorHeuristics(t *testing.T) {
	now := time.Now().UTC()
	svc := NewService(
		&fakeStateSource{},
		&fakeTMUX{
			snapshot: []tmuxctl.BoardPane{
				{
					PaneID:         "%1",
					SessionName:    "work",
					WindowIndex:    "1",
					WindowName:     "codex",
					PaneIndex:      "0",
					Command:        "codex-aarch64-a",
					Title:          "cdx",
					WindowActivity: now,
				},
				{
					PaneID:         "%2",
					SessionName:    "work",
					WindowIndex:    "2",
					WindowName:     "claude",
					PaneIndex:      "0",
					Command:        "claude",
					Title:          "claude",
					WindowActivity: now,
				},
			},
			captures: map[string]string{
				"%1": "some output\n› continue",
				"%2": "tool failed while applying patch",
			},
		},
		"",
	)

	rows, err := svc.Rows(context.Background())
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows len = %d, want 2", len(rows))
	}

	statuses := map[string]state.Status{}
	for _, row := range rows {
		statuses[row.PaneID] = row.Status
	}
	if statuses["%1"] != state.StatusDone {
		t.Fatalf("codex status = %s, want DONE", statuses["%1"])
	}
	if statuses["%2"] != state.StatusError {
		t.Fatalf("claude status = %s, want ERROR", statuses["%2"])
	}
}

func TestRowsRepoNameHandlesTrailingSlash(t *testing.T) {
	svc := NewService(
		&fakeStateSource{},
		&fakeTMUX{
			snapshot: []tmuxctl.BoardPane{
				{
					PaneID:      "%1",
					SessionName: "work",
					WindowIndex: "1",
					PaneIndex:   "0",
					Path:        "/tmp/panefleet/",
				},
			},
		},
		"",
	)

	rows, err := svc.Rows(context.Background())
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if rows[0].Repo != "panefleet" {
		t.Fatalf("rows[0].Repo = %q, want panefleet", rows[0].Repo)
	}
}

func intPtr(v int) *int {
	return &v
}
