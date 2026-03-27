package tmuxctl

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// ControlEvent describes one actionable tmux control-mode signal.
type ControlEvent struct {
	Kind string
	Raw  string
}

// ParseControlLine filters tmux control-mode lines to events that require a
// panefleet snapshot refresh.
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
	case "%layout-change",
		"%pane-mode-changed",
		"%pause",
		"%continue",
		"%session-changed",
		"%session-renamed",
		"%session-window-changed",
		"%sessions-changed",
		"%unlinked-window-add",
		"%unlinked-window-close",
		"%unlinked-window-renamed",
		"%window-add",
		"%window-close",
		"%window-pane-changed",
		"%window-renamed":
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
	var stderrTail string
	var stderrMu sync.Mutex
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			stderrMu.Lock()
			stderrTail = sc.Text()
			stderrMu.Unlock()
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
		<-stderrDone
		return withControlModeStderr(err, stderrTailSnapshot(&stderrMu, &stderrTail))
	}
	err = cmd.Wait()
	<-stderrDone
	if err != nil {
		return withControlModeStderr(err, stderrTailSnapshot(&stderrMu, &stderrTail))
	}
	return nil
}

func stderrTailSnapshot(mu *sync.Mutex, tail *string) string {
	mu.Lock()
	defer mu.Unlock()
	return strings.TrimSpace(*tail)
}

func withControlModeStderr(err error, stderrTail string) error {
	if stderrTail == "" {
		return err
	}
	return fmt.Errorf("%w (tmux stderr tail: %s)", err, stderrTail)
}
