package tmuxctl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWatchControlMode(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "tmux-fake")
	script := `#!/bin/sh
if [ "$1" = "-C" ]; then
  echo "%window-add @1"
  echo "%output %1 ignored"
  echo "%session-changed $1"
  exit 0
fi
exit 1
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	c := New(bin)
	got := make([]ControlEvent, 0, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.WatchControlMode(ctx, func(ev ControlEvent) {
		got = append(got, ev)
	}); err != nil {
		t.Fatalf("WatchControlMode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 actionable events, got %d (%#v)", len(got), got)
	}
	if got[0].Kind != "%window-add" || got[1].Kind != "%session-changed" {
		t.Fatalf("unexpected events: %#v", got)
	}
}

func TestWatchControlModeErrors(t *testing.T) {
	c := New("tmux")
	if err := c.WatchControlMode(context.Background(), nil); err == nil {
		t.Fatalf("expected nil callback error")
	}

	// Non-existing binary should fail to start.
	c = New(filepath.Join(t.TempDir(), "missing-tmux"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.WatchControlMode(ctx, func(ControlEvent) {}); err == nil {
		t.Fatalf("expected start error for missing binary")
	}
}

func TestWatchControlModeIncludesStderrTail(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "tmux-fake")
	script := `#!/bin/sh
if [ "$1" = "-C" ]; then
  echo "fatal-control-error" 1>&2
  exit 1
fi
exit 1
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}

	c := New(bin)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := c.WatchControlMode(ctx, func(ControlEvent) {})
	if err == nil {
		t.Fatalf("expected control-mode error")
	}
	if got := err.Error(); !strings.Contains(got, "tmux stderr tail: fatal-control-error") {
		t.Fatalf("expected stderr tail in error, got: %v", err)
	}
}
