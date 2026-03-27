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
	bin, err := resolveBinary(c.Binary)
	if err != nil {
		return "", err
	}
	// #nosec G204 -- tmux is an explicit operator-configured dependency and these arguments are fixed command verbs.
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", formatCommandError(bin, args, err, out)
	}
	return string(out), nil
}

func requirePaneID(paneID string) error {
	if strings.TrimSpace(paneID) == "" {
		return errors.New("pane id is required")
	}
	return nil
}

func resolveBinary(binary string) (string, error) {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		return "", errors.New("tmux binary is required")
	}
	return exec.LookPath(binary)
}

func formatCommandError(binary string, args []string, err error, output []byte) error {
	cmdLine := strings.TrimSpace(strings.Join(args, " "))
	outputText := strings.TrimSpace(string(output))
	if outputText == "" {
		return fmt.Errorf("%s %s failed: %w", binary, cmdLine, err)
	}
	return fmt.Errorf("%s %s failed: %w (%s)", binary, cmdLine, err, outputText)
}
