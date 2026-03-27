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

func TestBoardSnapshotAndPreview(t *testing.T) {
	bin, logPath := fakeTmux(t, `#!/bin/sh
echo "$@" >> "$PANEFLEET_FAKE_TMUX_LOG"
if [ "$1" = "list-panes" ]; then
  printf "%s\n" "%1	work	1	clean	0	codex-aarch64-a	cdx	/tmp/panefleet	0	0	1711533600		DONE	codex	1711533601	123	88"
  exit 0
fi
if [ "$1" = "display-message" ]; then
  printf "%s\n" "%1	work	1	clean	0	codex-aarch64-a	cdx	/tmp/panefleet	0		1711533600		DONE	codex	1711533601"
  exit 0
fi
if [ "$1" = "capture-pane" ]; then
  printf "%s\n" "hello"
  printf "%s\n" "world"
  exit 0
fi
if [ "$1" = "switch-client" ] || [ "$1" = "select-pane" ]; then
  exit 0
fi
exit 0
`)
	c := New(bin)

	rows, err := c.BoardSnapshot(context.Background())
	if err != nil {
		t.Fatalf("BoardSnapshot: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].PaneID != "%1" || rows[0].TokensUsed == nil || *rows[0].TokensUsed != 123 {
		t.Fatalf("unexpected board row: %+v", rows[0])
	}
	if rows[0].AgentStatus != "DONE" || rows[0].AgentTool != "codex" {
		t.Fatalf("unexpected agent metadata: %+v", rows[0])
	}
	if rows[0].ContextLeftPct == nil || *rows[0].ContextLeftPct != 88 {
		t.Fatalf("unexpected context_left_pct: %+v", rows[0])
	}
	if rows[0].WindowActivity.IsZero() {
		t.Fatalf("window activity should be parsed")
	}

	preview, err := c.Preview(context.Background(), "%1")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.Body != "hello\nworld" {
		t.Fatalf("preview body = %q, want hello\\nworld", preview.Body)
	}

	if err := c.JumpToPane(context.Background(), "%1", "work:1"); err != nil {
		t.Fatalf("JumpToPane: %v", err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	log := string(raw)
	if !strings.Contains(log, "switch-client -t work:1") {
		t.Fatalf("missing switch-client invocation: %s", log)
	}
	if !strings.Contains(log, "select-pane -t %1") {
		t.Fatalf("missing select-pane invocation: %s", log)
	}
}

func TestGlobalOption(t *testing.T) {
	bin, _ := fakeTmux(t, `#!/bin/sh
if [ "$1" = "show-options" ]; then
  printf "%s\n" "dracula"
  exit 0
fi
exit 0
`)
	c := New(bin)

	got, err := c.GlobalOption(context.Background(), "@panefleet-theme")
	if err != nil {
		t.Fatalf("GlobalOption: %v", err)
	}
	if got != "dracula" {
		t.Fatalf("GlobalOption = %q, want dracula", got)
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

func TestSnapshotAndOutputErrors(t *testing.T) {
	bin, _ := fakeTmux(t, `#!/bin/sh
if [ "$1" = "list-panes" ]; then
  printf "%s\n" "%1\tbad\t0"
  exit 0
fi
echo "boom" >&2
exit 2
`)
	c := New(bin)
	if _, err := c.Snapshot(context.Background()); err == nil {
		t.Fatalf("expected snapshot parse error")
	}

	if _, err := c.output(context.Background(), "kill-pane", "-t", "%1"); err == nil {
		t.Fatalf("expected output command error")
	}
}

func TestBoardSnapshotAndPreviewValidationErrors(t *testing.T) {
	bin, _ := fakeTmux(t, `#!/bin/sh
if [ "$1" = "list-panes" ]; then
  printf "%s\n" "%1	work	1	clean	0	codex	cdx	/tmp/panefleet	2	0	0					"
  exit 0
fi
if [ "$1" = "display-message" ]; then
  printf "%s\n" "%1\ttoo\tshort"
  exit 0
fi
exit 0
`)
	c := New(bin)
	if _, err := c.BoardSnapshot(context.Background()); err == nil {
		t.Fatalf("expected BoardSnapshot parse error")
	}

	bin, _ = fakeTmux(t, `#!/bin/sh
if [ "$1" = "display-message" ]; then
  printf "%s\n" "%1\ttoo\tshort"
  exit 0
fi
if [ "$1" = "capture-pane" ]; then
  exit 0
fi
exit 0
`)
	c = New(bin)
	if _, err := c.Preview(context.Background(), "%1"); err == nil {
		t.Fatalf("expected Preview parse error")
	}
	if err := c.JumpToPane(context.Background(), "%1", ""); err == nil {
		t.Fatalf("expected JumpToPane target validation error")
	}
}

func TestNewDefaultBinaryAndSnapshotCommandError(t *testing.T) {
	if got := New("").Binary; got != "tmux" {
		t.Fatalf("New(\"\").Binary = %q, want tmux", got)
	}
	if got := New("  ").Binary; got != "tmux" {
		t.Fatalf("New(\"  \").Binary = %q, want tmux", got)
	}

	bin, _ := fakeTmux(t, `#!/bin/sh
if [ "$1" = "list-panes" ]; then
  echo "cannot list" >&2
  exit 2
fi
exit 0
`)
	c := New(bin)
	if _, err := c.Snapshot(context.Background()); err == nil {
		t.Fatalf("expected snapshot command error")
	}
}

func TestFormatCommandErrorOmitsEmptyOutput(t *testing.T) {
	err := formatCommandError("tmux", []string{"list-panes"}, context.DeadlineExceeded, nil)
	if strings.Contains(err.Error(), "()") {
		t.Fatalf("unexpected empty output marker in error: %v", err)
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
