package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alnah/panefleet/internal/state"
)

func TestCLIEndToEndSyncAndStateCommands(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "panefleet.db")
	fakeTMUX := writeFakeTmux(t)
	t.Setenv("PANEFLEET_DB_PATH", dbPath)
	t.Setenv("PANEFLEET_TMUX_BIN", fakeTMUX)

	if err := run(context.Background(), []string{"sync-tmux"}); err != nil {
		t.Fatalf("sync-tmux: %v", err)
	}

	rawList := captureStdout(t, func() {
		if err := run(context.Background(), []string{"state-list"}); err != nil {
			t.Fatalf("state-list: %v", err)
		}
	})
	var list []state.PaneState
	if err := json.Unmarshal([]byte(rawList), &list); err != nil {
		t.Fatalf("decode state-list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(list))
	}

	// Dead pane from fake tmux should map to ERROR.
	var dead state.PaneState
	foundDead := false
	for _, item := range list {
		if item.PaneID == "%12" {
			dead = item
			foundDead = true
			break
		}
	}
	if !foundDead {
		t.Fatalf("pane %%12 not found in %+v", list)
	}
	if dead.Status != state.StatusError {
		t.Fatalf("expected pane %%12 ERROR, got %s", dead.Status)
	}

	if err := run(context.Background(), []string{"state-set", "--pane", "%11", "--status", "STALE"}); err != nil {
		t.Fatalf("state-set: %v", err)
	}

	rawShow := captureStdout(t, func() {
		if err := run(context.Background(), []string{"state-show", "--pane", "%11"}); err != nil {
			t.Fatalf("state-show: %v", err)
		}
	})
	var shown state.PaneState
	if err := json.Unmarshal([]byte(rawShow), &shown); err != nil {
		t.Fatalf("decode state-show: %v", err)
	}
	if shown.Status != state.StatusStale {
		t.Fatalf("expected STALE after override, got %s", shown.Status)
	}

	if err := run(context.Background(), []string{"state-clear", "--pane", "%11"}); err != nil {
		t.Fatalf("state-clear: %v", err)
	}
}

func TestCLIEndToEndPaneOps(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "panefleet.db")
	fakeTMUX := writeFakeTmux(t)
	t.Setenv("PANEFLEET_DB_PATH", dbPath)
	t.Setenv("PANEFLEET_TMUX_BIN", fakeTMUX)
	logPath := os.Getenv("PANEFLEET_FAKE_TMUX_LOG")

	if err := run(context.Background(), []string{"pane-kill", "--pane", "%90"}); err != nil {
		t.Fatalf("pane-kill: %v", err)
	}
	if err := run(context.Background(), []string{"pane-respawn", "--pane", "%90"}); err != nil {
		t.Fatalf("pane-respawn: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read tmux log: %v", err)
	}
	log := string(raw)
	if !strings.Contains(log, "kill-pane -t %90") {
		t.Fatalf("missing kill-pane call: %s", log)
	}
	if !strings.Contains(log, "respawn-pane -k -t %90") {
		t.Fatalf("missing respawn-pane call: %s", log)
	}
}

func writeFakeTmux(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "tmux.log")
	t.Setenv("PANEFLEET_FAKE_TMUX_LOG", logPath)
	bin := filepath.Join(dir, "tmux-fake")
	script := `#!/bin/sh
echo "$@" >> "$PANEFLEET_FAKE_TMUX_LOG"
if [ "$1" = "list-panes" ]; then
  printf "%s\n" "%11	0	0"
  printf "%s\n" "%12	1	2"
  exit 0
fi
if [ "$1" = "-C" ]; then
  # control-mode watcher test path
  echo "%window-add @1"
  exit 0
fi
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	return bin
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	_ = w.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return buf.String()
}
