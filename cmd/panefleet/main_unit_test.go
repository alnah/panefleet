package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alnah/panefleet/internal/state"
	"github.com/alnah/panefleet/internal/tmuxctl"
)

type fakeTeaProgram struct {
	err error
}

func (f fakeTeaProgram) Run() (tea.Model, error) {
	return nil, f.err
}

type delayedFakeTeaProgram struct {
	wait time.Duration
	err  error
}

func (f delayedFakeTeaProgram) Run() (tea.Model, error) {
	time.Sleep(f.wait)
	return nil, f.err
}

func TestParseKindAndUsage(t *testing.T) {
	kinds := map[string]state.EventKind{
		"start":          state.EventPaneStarted,
		"wait":           state.EventPaneWaiting,
		"exit":           state.EventPaneExited,
		"observe":        state.EventPaneObserved,
		"override-set":   state.EventOverrideSet,
		"override-clear": state.EventOverrideCleared,
		"tick":           state.EventTimerRecompute,
	}
	for raw, want := range kinds {
		got, err := parseKind(raw)
		if err != nil {
			t.Fatalf("parseKind(%q): %v", raw, err)
		}
		if got != want {
			t.Fatalf("parseKind(%q) = %q, want %q", raw, got, want)
		}
	}
	if _, err := parseKind("nope"); err == nil {
		t.Fatalf("parseKind should fail on invalid kind")
	}
	if err := usageError(); err == nil || !strings.Contains(err.Error(), "usage: panefleet") {
		t.Fatalf("usageError unexpected: %v", err)
	}
}

func TestShouldManageDBPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"", false},
		{":memory:", false},
		{"file::memory:?cache=shared", false},
		{"file:/tmp/panefleet.db?cache=shared", false},
		{"/tmp/panefleet.db", true},
		{"/tmp/pane?fleet.db", false},
	}
	for _, tc := range cases {
		if got := shouldManageDBPath(tc.path); got != tc.want {
			t.Fatalf("shouldManageDBPath(%q)=%v want=%v", tc.path, got, tc.want)
		}
	}
}

func TestValidatePositiveDuration(t *testing.T) {
	if err := validatePositiveDuration("x", time.Millisecond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := validatePositiveDuration("x", 0); err == nil {
		t.Fatalf("expected error for zero duration")
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	fn()
	_ = w.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return buf.String()
}

func TestRuntimeAPIHappyPath(t *testing.T) {
	svc, closer, err := newService(context.Background())
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	defer closer()

	// Use a fake tmux from existing helper in e2e test.
	fakeTMUX := writeFakeTmux(t)
	t.Setenv("PANEFLEET_TMUX_BIN", fakeTMUX)
	api := runtimeAPI{svc: svc, tmux: tmuxctl.New(fakeTMUX)}

	now := time.Now().UTC()
	if _, err := svc.Ingest(context.Background(), state.Event{
		PaneID:     "%11",
		Kind:       state.EventPaneStarted,
		OccurredAt: now,
	}); err != nil {
		t.Fatalf("ingest start: %v", err)
	}

	states, err := api.StateList(context.Background())
	if err != nil || len(states) == 0 {
		t.Fatalf("StateList failed: len=%d err=%v", len(states), err)
	}

	if _, err := api.SetOverride(context.Background(), "%11", state.StatusStale, "test"); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	if _, err := api.ClearOverride(context.Background(), "%11", "test"); err != nil {
		t.Fatalf("ClearOverride: %v", err)
	}
	if err := api.KillPane(context.Background(), "%11"); err != nil {
		t.Fatalf("KillPane: %v", err)
	}
	if err := api.RespawnPane(context.Background(), "%11"); err != nil {
		t.Fatalf("RespawnPane: %v", err)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	t.Setenv("PANEFLEET_DB_PATH", "/dev/null/panefleet.db")
	err := run(context.Background(), []string{"unknown"})
	if err == nil || !strings.Contains(err.Error(), "usage: panefleet") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

func TestRunPaneCommandsDoNotRequireServiceBootstrap(t *testing.T) {
	fakeTMUX := writeFakeTmux(t)
	t.Setenv("PANEFLEET_TMUX_BIN", fakeTMUX)
	t.Setenv("PANEFLEET_DB_PATH", "/dev/null/panefleet.db")

	if err := run(context.Background(), []string{"pane-kill", "--pane", "%90"}); err != nil {
		t.Fatalf("pane-kill should not require DB bootstrap: %v", err)
	}
	if err := run(context.Background(), []string{"pane-respawn", "--pane", "%90"}); err != nil {
		t.Fatalf("pane-respawn should not require DB bootstrap: %v", err)
	}
}

func TestCmdIngestKindsAndErrors(t *testing.T) {
	svc, closer, err := newService(context.Background())
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	defer closer()

	if err := cmdIngest(context.Background(), svc, []string{"--pane", "%1", "--kind", "start"}); err != nil {
		t.Fatalf("cmdIngest start: %v", err)
	}
	if err := cmdIngest(context.Background(), svc, []string{"--pane", "%1", "--kind", "wait"}); err != nil {
		t.Fatalf("cmdIngest wait: %v", err)
	}
	if err := cmdIngest(context.Background(), svc, []string{"--pane", "%1", "--kind", "exit", "--exit-code", "0"}); err != nil {
		t.Fatalf("cmdIngest exit: %v", err)
	}
	if err := cmdIngest(context.Background(), svc, []string{"--pane", "%2", "--kind", "override-set", "--override", "STALE"}); err != nil {
		t.Fatalf("cmdIngest override-set: %v", err)
	}
	if err := cmdIngest(context.Background(), svc, []string{"--pane", "%2", "--kind", "override-clear"}); err != nil {
		t.Fatalf("cmdIngest override-clear: %v", err)
	}
	if err := cmdIngest(context.Background(), svc, []string{"--pane", "%2", "--kind", "tick"}); err != nil {
		t.Fatalf("cmdIngest tick: %v", err)
	}

	if err := cmdIngest(context.Background(), svc, []string{"--pane", "%3", "--kind", "override-set", "--override", "NOPE"}); err == nil {
		t.Fatalf("expected override parse error")
	}
	if err := cmdIngest(context.Background(), svc, []string{"--pane", "%3", "--kind", "start", "--at", "invalid"}); err == nil {
		t.Fatalf("expected invalid --at parse error")
	}
}

func TestStateCommandsAndSyncErrors(t *testing.T) {
	svc, closer, err := newService(context.Background())
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	defer closer()

	now := time.Now().UTC()
	if _, err := svc.Ingest(context.Background(), state.Event{
		PaneID:     "%11",
		Kind:       state.EventPaneStarted,
		OccurredAt: now,
	}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	if err := cmdStateShow(context.Background(), svc, []string{"--pane", "%11"}); err != nil {
		t.Fatalf("cmdStateShow: %v", err)
	}
	if err := cmdStateList(context.Background(), svc, nil); err != nil {
		t.Fatalf("cmdStateList: %v", err)
	}
	if err := cmdStateSet(context.Background(), svc, []string{"--pane", "%11", "--status", "STALE"}); err != nil {
		t.Fatalf("cmdStateSet: %v", err)
	}
	if err := cmdStateClear(context.Background(), svc, []string{"--pane", "%11"}); err != nil {
		t.Fatalf("cmdStateClear: %v", err)
	}

	if err := cmdStateSet(context.Background(), svc, []string{"--pane", "%11", "--status", "NOPE"}); err == nil {
		t.Fatalf("expected cmdStateSet parse status error")
	}
	if err := cmdStateShow(context.Background(), svc, []string{"--unknown"}); err == nil {
		t.Fatalf("expected cmdStateShow parse flag error")
	}
	if err := cmdStateList(context.Background(), svc, []string{"--unknown"}); err == nil {
		t.Fatalf("expected cmdStateList parse flag error")
	}
	if err := cmdStateClear(context.Background(), svc, []string{"--unknown"}); err == nil {
		t.Fatalf("expected cmdStateClear parse flag error")
	}
	if err := cmdSyncTmux(context.Background(), svc, []string{"--at", "invalid"}); err == nil {
		t.Fatalf("expected invalid --at parse error")
	}
}

func TestPaneCommandsErrorBranches(t *testing.T) {
	fakeTMUX := writeFakeTmux(t)
	t.Setenv("PANEFLEET_TMUX_BIN", fakeTMUX)

	if err := cmdPaneKill(context.Background(), []string{"--pane", ""}); err == nil {
		t.Fatalf("expected pane-kill validation error")
	}
	if err := cmdPaneRespawn(context.Background(), []string{"--pane", ""}); err == nil {
		t.Fatalf("expected pane-respawn validation error")
	}
}

func TestRunValidationErrors(t *testing.T) {
	svc, closer, err := newService(context.Background())
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	defer closer()
	if err := cmdRun(context.Background(), svc, []string{"--refresh", "0s", "--control-mode=false"}); err == nil {
		t.Fatalf("expected refresh validation error")
	}
	if err := cmdRun(context.Background(), svc, []string{"--sync-every", "0s", "--control-mode=false"}); err == nil {
		t.Fatalf("expected sync validation error")
	}
}

func TestCmdTUIAndRunRequireTMUXSession(t *testing.T) {
	svc, closer, err := newService(context.Background())
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	defer closer()

	t.Setenv("TMUX", "")
	if err := cmdTUI(svc, []string{"--refresh", "10ms"}); err == nil || !strings.Contains(err.Error(), "inside tmux") {
		t.Fatalf("expected tmux session error for tui, got %v", err)
	}
	if err := cmdRun(context.Background(), svc, []string{"--refresh", "10ms", "--sync-every", "10ms", "--control-mode=false"}); err == nil || !strings.Contains(err.Error(), "inside tmux") {
		t.Fatalf("expected tmux session error for run, got %v", err)
	}
}

func TestCLIRejectsUnexpectedArgs(t *testing.T) {
	svc, closer, err := newService(context.Background())
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	defer closer()

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "ingest",
			run: func() error {
				return cmdIngest(context.Background(), svc, []string{"--pane", "%1", "--kind", "start", "extra"})
			},
		},
		{
			name: "state-show",
			run: func() error {
				return cmdStateShow(context.Background(), svc, []string{"--pane", "%1", "extra"})
			},
		},
		{
			name: "state-list",
			run: func() error {
				return cmdStateList(context.Background(), svc, []string{"extra"})
			},
		},
		{
			name: "state-set",
			run: func() error {
				return cmdStateSet(context.Background(), svc, []string{"--pane", "%1", "--status", "RUN", "extra"})
			},
		},
		{
			name: "state-clear",
			run: func() error {
				return cmdStateClear(context.Background(), svc, []string{"--pane", "%1", "extra"})
			},
		},
		{
			name: "sync-tmux",
			run: func() error {
				return cmdSyncTmux(context.Background(), svc, []string{"extra"})
			},
		},
		{
			name: "tui",
			run: func() error {
				return cmdTUI(svc, []string{"extra"})
			},
		},
		{
			name: "run",
			run: func() error {
				return cmdRun(context.Background(), svc, []string{"--control-mode=false", "extra"})
			},
		},
		{
			name: "pane-kill",
			run: func() error {
				return cmdPaneKill(context.Background(), []string{"--pane", "%1", "extra"})
			},
		},
		{
			name: "pane-respawn",
			run: func() error {
				return cmdPaneRespawn(context.Background(), []string{"--pane", "%1", "extra"})
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil || !strings.Contains(err.Error(), "unexpected arguments") {
				t.Fatalf("expected unexpected arguments error, got: %v", err)
			}
		})
	}
}

func TestNewServiceErrorPathAndSyncTmuxOnceIngestError(t *testing.T) {
	// MkdirAll should fail with non-directory parent.
	t.Setenv("PANEFLEET_DB_PATH", "/dev/null/panefleet.db")
	if _, _, err := newService(context.Background()); err == nil {
		t.Fatalf("expected newService path error")
	}
	t.Setenv("PANEFLEET_DB_PATH", filepath.Join(t.TempDir(), "panefleet.db"))

	svc, closer, err := newService(context.Background())
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	defer closer()

	late := time.Now().UTC().Add(5 * time.Minute)
	if _, err := svc.Ingest(context.Background(), state.Event{
		PaneID:     "%11",
		Kind:       state.EventPaneObserved,
		OccurredAt: late,
	}); err != nil {
		t.Fatalf("ingest setup: %v", err)
	}

	fakeTMUX := writeFakeTmux(t)
	tmux := tmuxctl.New(fakeTMUX)
	if _, err := syncTmuxOnce(context.Background(), svc, tmux, time.Now().UTC(), "adapter:test"); err == nil {
		t.Fatalf("expected syncTmuxOnce ingest out-of-order error")
	}
}

func TestCmdTUIAndRunWithInjectedProgram(t *testing.T) {
	svc, closer, err := newService(context.Background())
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	defer closer()
	t.Setenv("TMUX", "/tmp/tmux-test")

	oldFactory := newTeaProgram
	newTeaProgram = func(_ tea.Model, _ ...tea.ProgramOption) teaProgram {
		return fakeTeaProgram{}
	}
	defer func() { newTeaProgram = oldFactory }()

	if err := cmdTUI(svc, []string{"--refresh", "10ms"}); err != nil {
		t.Fatalf("cmdTUI: %v", err)
	}

	fakeTMUX := writeFakeTmux(t)
	t.Setenv("PANEFLEET_TMUX_BIN", fakeTMUX)
	if err := cmdRun(context.Background(), svc, []string{"--refresh", "10ms", "--sync-every", "10ms", "--control-mode=false"}); err != nil {
		t.Fatalf("cmdRun: %v", err)
	}
}

func TestCmdRunControlModePath(t *testing.T) {
	svc, closer, err := newService(context.Background())
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	defer closer()
	t.Setenv("TMUX", "/tmp/tmux-test")

	oldFactory := newTeaProgram
	newTeaProgram = func(_ tea.Model, _ ...tea.ProgramOption) teaProgram {
		return delayedFakeTeaProgram{wait: 40 * time.Millisecond}
	}
	defer func() { newTeaProgram = oldFactory }()

	fakeTMUX := writeFakeTmux(t)
	t.Setenv("PANEFLEET_TMUX_BIN", fakeTMUX)
	if err := cmdRun(context.Background(), svc, []string{
		"--refresh", "10ms",
		"--sync-every", "10ms",
		"--control-mode=true",
	}); err != nil {
		t.Fatalf("cmdRun control mode: %v", err)
	}
}

func TestCmdRunLogsSyncErrors(t *testing.T) {
	svc, closer, err := newService(context.Background())
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	defer closer()
	t.Setenv("TMUX", "/tmp/tmux-test")

	oldFactory := newTeaProgram
	newTeaProgram = func(_ tea.Model, _ ...tea.ProgramOption) teaProgram {
		return delayedFakeTeaProgram{wait: 30 * time.Millisecond}
	}
	defer func() { newTeaProgram = oldFactory }()

	t.Setenv("PANEFLEET_TMUX_BIN", filepath.Join(t.TempDir(), "missing-tmux"))
	output := captureStderr(t, func() {
		if err := cmdRun(context.Background(), svc, []string{
			"--refresh", "10ms",
			"--sync-every", "10ms",
			"--control-mode=false",
		}); err != nil {
			t.Fatalf("cmdRun: %v", err)
		}
	})
	if !strings.Contains(output, "sync.initial") {
		t.Fatalf("expected sync.initial log, got: %q", output)
	}
	if !strings.Contains(output, "run_id=") {
		t.Fatalf("expected run_id in logs, got: %q", output)
	}
}

func TestParseOptionalRFC3339TimeAndRunVerbose(t *testing.T) {
	if _, err := parseOptionalRFC3339Time("bad"); err == nil {
		t.Fatalf("expected parseOptionalRFC3339Time error")
	}
	if ts, err := parseOptionalRFC3339Time(""); err != nil || ts.IsZero() {
		t.Fatalf("expected current time fallback, ts=%v err=%v", ts, err)
	}

	t.Setenv("PANEFLEET_OBS_VERBOSE", "true")
	if !runVerboseEnabled() {
		t.Fatalf("expected verbose=true")
	}
	t.Setenv("PANEFLEET_OBS_VERBOSE", "0")
	if runVerboseEnabled() {
		t.Fatalf("expected verbose=false")
	}
}

func TestResolveBoardTheme(t *testing.T) {
	t.Setenv("PANEFLEET_THEME", "rose-pine")
	if got := resolveBoardTheme(context.Background(), nil); got != "rose-pine" {
		t.Fatalf("resolveBoardTheme env = %q, want rose-pine", got)
	}

	t.Setenv("PANEFLEET_THEME", "")
	fakeTMUX := filepath.Join(t.TempDir(), "tmux-theme")
	if err := os.WriteFile(fakeTMUX, []byte(`#!/bin/sh
if [ "$1" = "show-options" ]; then
  printf "%s\n" "dracula"
  exit 0
fi
exit 0
`), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	tmux := tmuxctl.New(fakeTMUX)
	if got := resolveBoardTheme(context.Background(), tmux); got != "dracula" {
		t.Fatalf("resolveBoardTheme tmux = %q, want dracula", got)
	}
}

func TestNewServiceCreatesSecureDBPath(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "state", "panefleet.db")
	t.Setenv("PANEFLEET_DB_PATH", dbPath)

	_, closer, err := newService(context.Background())
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	defer closer()

	info, err := os.Stat(filepath.Dir(dbPath))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("db dir mode = %o, want 700", info.Mode().Perm())
	}
}

func TestResolveDBPathHonorsXDGStateHome(t *testing.T) {
	t.Setenv("PANEFLEET_DB_PATH", "")
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "xdg-state"))

	got, err := resolveDBPath()
	if err != nil {
		t.Fatalf("resolveDBPath: %v", err)
	}
	want := filepath.Join(os.Getenv("XDG_STATE_HOME"), "panefleet", "panefleet.db")
	if got != want {
		t.Fatalf("resolveDBPath = %q, want %q", got, want)
	}
}

func TestNewServiceSkipsFilesystemPrepForSQLiteURI(t *testing.T) {
	t.Setenv("PANEFLEET_DB_PATH", "file::memory:?cache=shared")

	_, closer, err := newService(context.Background())
	if err != nil {
		t.Fatalf("newService with sqlite URI: %v", err)
	}
	defer closer()
}
