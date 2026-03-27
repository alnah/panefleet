package tmuxctl

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/alnah/panefleet/internal/tmuxsync"
)

type ExecClient struct {
	Binary string
}

func New(binary string) *ExecClient {
	if binary == "" {
		binary = "tmux"
	}
	return &ExecClient{Binary: binary}
}

func (c *ExecClient) Snapshot(ctx context.Context) ([]tmuxsync.PaneSnapshot, error) {
	out, err := c.output(ctx, "list-panes", "-a", "-F", tmuxsync.ListPanesFormat)
	if err != nil {
		return nil, err
	}
	return tmuxsync.ParseListPanesOutput(out)
}

func (c *ExecClient) KillPane(ctx context.Context, paneID string) error {
	if strings.TrimSpace(paneID) == "" {
		return fmt.Errorf("pane id is required")
	}
	_, err := c.output(ctx, "kill-pane", "-t", paneID)
	return err
}

func (c *ExecClient) RespawnPane(ctx context.Context, paneID string) error {
	if strings.TrimSpace(paneID) == "" {
		return fmt.Errorf("pane id is required")
	}
	_, err := c.output(ctx, "respawn-pane", "-k", "-t", paneID)
	return err
}

func (c *ExecClient) output(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s failed: %w (%s)", c.Binary, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
