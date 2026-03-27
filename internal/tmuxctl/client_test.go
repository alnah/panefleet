package tmuxctl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotParsesListPanes(t *testing.T) {
	bin, _ := fakeTmux(t, `#!/bin/sh
if [ "$1" = "list-panes" ]; then
  printf "%s\n" "%1	0	0"
  printf "%s\n" "%2	1	2"
  exit 0
fi
exit 0
`)
	c := New(bin)
	out, err := c.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(out))
	}
	if out[1].PaneID != "%2" || !out[1].Dead || out[1].DeadStatus != 2 {
		t.Fatalf("unexpected pane 2: %+v", out[1])
	}
}

func TestKillAndRespawnInvokeTmux(t *testing.T) {
	bin, logPath := fakeTmux(t, `#!/bin/sh
echo "$@" >> "$PANEFLEET_FAKE_TMUX_LOG"
exit 0
`)
	c := New(bin)

	if err := c.KillPane(context.Background(), "%9"); err != nil {
		t.Fatalf("kill pane: %v", err)
	}
	if err := c.RespawnPane(context.Background(), "%9"); err != nil {
		t.Fatalf("respawn pane: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	log := string(raw)
	if !strings.Contains(log, "kill-pane -t %9") {
		t.Fatalf("missing kill-pane invocation in log: %s", log)
	}
	if !strings.Contains(log, "respawn-pane -k -t %9") {
		t.Fatalf("missing respawn-pane invocation in log: %s", log)
	}
}

func TestInvalidPaneID(t *testing.T) {
	c := New("tmux")
	if err := c.KillPane(context.Background(), ""); err == nil {
		t.Fatalf("expected kill-pane validation error")
	}
	if err := c.RespawnPane(context.Background(), " "); err == nil {
		t.Fatalf("expected respawn-pane validation error")
	}
}

func fakeTmux(t *testing.T, script string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "tmux-fake")
	logPath := filepath.Join(dir, "tmux.log")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PANEFLEET_FAKE_TMUX_LOG", logPath)
	return bin, logPath
}
