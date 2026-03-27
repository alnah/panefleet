package tmuxctl

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type ControlEvent struct {
	Kind string
	Raw  string
}

func ParseControlLine(line string) (ControlEvent, bool) {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "%") {
		return ControlEvent{}, false
	}
	kind := line
	if idx := strings.IndexByte(line, ' '); idx >= 0 {
		kind = line[:idx]
	}
	switch kind {
	case "%layout-change", "%window-add", "%window-close", "%window-renamed", "%session-changed", "%session-window-changed", "%pane-mode-changed", "%unlinked-window-close":
		return ControlEvent{Kind: kind, Raw: line}, true
	default:
		return ControlEvent{}, false
	}
}

// WatchControlMode streams tmux control-mode output and invokes onEvent for actionable lines.
// It returns when context is done or when tmux exits.
func (c *ExecClient) WatchControlMode(ctx context.Context, onEvent func(ControlEvent)) error {
	if onEvent == nil {
		return fmt.Errorf("onEvent callback is required")
	}
	args := []string{"-C", "attach-session"}
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Drain stderr to avoid potential blocking.
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			_ = sc.Text()
		}
	}()

	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		ev, ok := ParseControlLine(sc.Text())
		if ok {
			onEvent(ev)
		}
	}
	if err := sc.Err(); err != nil {
		_ = cmd.Wait()
		return err
	}
	return cmd.Wait()
}
