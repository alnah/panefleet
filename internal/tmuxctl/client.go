package tmuxctl

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/alnah/panefleet/internal/tmuxsync"
)

// ExecClient issues tmux commands through the configured binary.
type ExecClient struct {
	Binary string
}

// New returns an ExecClient bound to a tmux binary path.
func New(binary string) *ExecClient {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "tmux"
	}
	return &ExecClient{Binary: binary}
}

// Snapshot captures current tmux pane metadata used by the reducer adapter.
func (c *ExecClient) Snapshot(ctx context.Context) ([]tmuxsync.PaneSnapshot, error) {
	out, err := c.output(ctx, "list-panes", "-a", "-F", tmuxsync.ListPanesFormat)
	if err != nil {
		return nil, err
	}
	return tmuxsync.ParseListPanesOutput(out)
}

// KillPane requests tmux to terminate one pane by id.
func (c *ExecClient) KillPane(ctx context.Context, paneID string) error {
	if err := requirePaneID(paneID); err != nil {
		return err
	}
	_, err := c.output(ctx, "kill-pane", "-t", paneID)
	return err
}

// RespawnPane requests tmux to restart one pane in-place.
func (c *ExecClient) RespawnPane(ctx context.Context, paneID string) error {
	if err := requirePaneID(paneID); err != nil {
		return err
	}
	_, err := c.output(ctx, "respawn-pane", "-k", "-t", paneID)
	return err
}

func (c *ExecClient) output(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", formatCommandError(c.Binary, args, err, out)
	}
	return string(out), nil
}

func requirePaneID(paneID string) error {
	if strings.TrimSpace(paneID) == "" {
		return errors.New("pane id is required")
	}
	return nil
}

func formatCommandError(binary string, args []string, err error, output []byte) error {
	cmdLine := strings.TrimSpace(strings.Join(args, " "))
	outputText := strings.TrimSpace(string(output))
	if outputText == "" {
		return fmt.Errorf("%s %s failed: %w", binary, cmdLine, err)
	}
	return fmt.Errorf("%s %s failed: %w (%s)", binary, cmdLine, err, outputText)
}
