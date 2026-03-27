package tmuxctl

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/alnah/panefleet/internal/tmuxsync"
)

// ExecClient issues tmux commands through the configured binary.
type ExecClient struct {
	Binary string
}

// BoardPane is the rich tmux row used by the Go board.
type BoardPane struct {
	PaneID         string
	SessionName    string
	WindowIndex    string
	WindowName     string
	PaneIndex      string
	Command        string
	Title          string
	Path           string
	Dead           bool
	DeadStatus     int
	WindowActivity time.Time
	LocalStatus    string
	AgentStatus    string
	AgentTool      string
	AgentUpdatedAt time.Time
	TokensUsed     *int
	ContextLeftPct *int
}

// PanePreview is the live preview payload used by the Go board.
type PanePreview struct {
	PaneID         string
	SessionName    string
	WindowIndex    string
	WindowName     string
	PaneIndex      string
	Command        string
	Title          string
	Path           string
	Dead           bool
	DeadStatus     int
	WindowActivity time.Time
	LocalStatus    string
	AgentStatus    string
	AgentTool      string
	AgentUpdatedAt time.Time
	Body           string
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

// GlobalOption reads one global tmux option value and trims its trailing line
// ending so callers can use it directly in Go logic.
func (c *ExecClient) GlobalOption(ctx context.Context, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("option name is required")
	}
	out, err := c.output(ctx, "show-options", "-gqv", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// BoardSnapshot captures the richer pane metadata required by the Go board.
func (c *ExecClient) BoardSnapshot(ctx context.Context) ([]BoardPane, error) {
	out, err := c.output(ctx, "list-panes", "-a", "-F", boardListPanesFormat)
	if err != nil {
		return nil, err
	}
	return parseBoardSnapshotOutput(out)
}

// Preview returns current pane metadata plus visible capture for the Go board.
func (c *ExecClient) Preview(ctx context.Context, paneID string) (PanePreview, error) {
	if err := requirePaneID(paneID); err != nil {
		return PanePreview{}, err
	}
	meta, err := c.output(ctx, "display-message", "-p", "-t", paneID, boardPreviewFormat)
	if err != nil {
		return PanePreview{}, err
	}
	preview, err := parsePanePreview(strings.TrimRight(meta, "\r\n"))
	if err != nil {
		return PanePreview{}, err
	}
	body, err := c.output(ctx, "capture-pane", "-p", "-t", paneID)
	if err != nil {
		return PanePreview{}, err
	}
	preview.Body = strings.TrimRight(body, "\n")
	return preview, nil
}

// RecentCapture returns a bounded recent pane capture for lightweight board
// heuristics.
func (c *ExecClient) RecentCapture(ctx context.Context, paneID string) (string, error) {
	if err := requirePaneID(paneID); err != nil {
		return "", err
	}
	out, err := c.output(ctx, "capture-pane", "-p", "-t", paneID, "-S", "-30")
	if err != nil {
		return "", err
	}
	return strings.TrimRight(out, "\n"), nil
}

// JumpToPane switches to the target window and focuses the target pane.
func (c *ExecClient) JumpToPane(ctx context.Context, paneID, targetWindow string) error {
	if err := requirePaneID(paneID); err != nil {
		return err
	}
	if strings.TrimSpace(targetWindow) == "" {
		return errors.New("target window is required")
	}
	if _, err := c.output(ctx, "switch-client", "-t", targetWindow); err != nil {
		return err
	}
	_, err := c.output(ctx, "select-pane", "-t", paneID)
	return err
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

const (
	boardListPanesFormat = "#{pane_id}\t#{session_name}\t#{window_index}\t#{window_name}\t#{pane_index}\t#{pane_current_command}\t#{pane_title}\t#{pane_current_path}\t#{pane_dead}\t#{pane_dead_status}\t#{window_activity}\t#{@panefleet_status}\t#{@panefleet_agent_status}\t#{@panefleet_agent_tool}\t#{@panefleet_agent_updated_at}\t#{@panefleet_tokens_used}\t#{@panefleet_context_left_pct}"
	boardPreviewFormat   = "#{pane_id}\t#{session_name}\t#{window_index}\t#{window_name}\t#{pane_index}\t#{pane_current_command}\t#{pane_title}\t#{pane_current_path}\t#{pane_dead}\t#{pane_dead_status}\t#{window_activity}\t#{@panefleet_status}\t#{@panefleet_agent_status}\t#{@panefleet_agent_tool}\t#{@panefleet_agent_updated_at}"
)

func parseBoardSnapshotOutput(raw string) ([]BoardPane, error) {
	lines := strings.Split(raw, "\n")
	out := make([]BoardPane, 0, len(lines))
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		pane, err := parseBoardPane(line, i+1)
		if err != nil {
			return nil, err
		}
		out = append(out, pane)
	}
	return out, nil
}

func parseBoardPane(line string, lineNo int) (BoardPane, error) {
	parts := strings.Split(line, "\t")
	if len(parts) != 17 {
		return BoardPane{}, fmt.Errorf("line %d: expected 17 columns, got %d", lineNo, len(parts))
	}
	if strings.TrimSpace(parts[0]) == "" {
		return BoardPane{}, fmt.Errorf("line %d: pane_id is required", lineNo)
	}
	deadValue, err := strconv.Atoi(parts[8])
	if err != nil {
		return BoardPane{}, fmt.Errorf("line %d: invalid pane_dead: %w", lineNo, err)
	}
	if deadValue != 0 && deadValue != 1 {
		return BoardPane{}, fmt.Errorf("line %d: invalid pane_dead value %d", lineNo, deadValue)
	}
	deadStatus := 0
	if strings.TrimSpace(parts[9]) != "" {
		deadStatus, err = strconv.Atoi(parts[9])
		if err != nil {
			return BoardPane{}, fmt.Errorf("line %d: invalid pane_dead_status: %w", lineNo, err)
		}
	}
	if deadStatus < 0 {
		return BoardPane{}, fmt.Errorf("line %d: invalid pane_dead_status value %d", lineNo, deadStatus)
	}
	activity, err := parseOptionalUnixTime(parts[10])
	if err != nil {
		return BoardPane{}, fmt.Errorf("line %d: invalid window_activity: %w", lineNo, err)
	}
	agentUpdatedAt, err := parseOptionalUnixTime(parts[14])
	if err != nil {
		return BoardPane{}, fmt.Errorf("line %d: invalid agent_updated_at: %w", lineNo, err)
	}
	tokensUsed, err := parseOptionalInt(parts[15])
	if err != nil {
		return BoardPane{}, fmt.Errorf("line %d: invalid tokens_used: %w", lineNo, err)
	}
	contextLeftPct, err := parseOptionalInt(parts[16])
	if err != nil {
		return BoardPane{}, fmt.Errorf("line %d: invalid context_left_pct: %w", lineNo, err)
	}

	return BoardPane{
		PaneID:         parts[0],
		SessionName:    parts[1],
		WindowIndex:    parts[2],
		WindowName:     parts[3],
		PaneIndex:      parts[4],
		Command:        parts[5],
		Title:          parts[6],
		Path:           parts[7],
		Dead:           deadValue == 1,
		DeadStatus:     deadStatus,
		WindowActivity: activity,
		LocalStatus:    strings.TrimSpace(parts[11]),
		AgentStatus:    strings.TrimSpace(parts[12]),
		AgentTool:      strings.TrimSpace(parts[13]),
		AgentUpdatedAt: agentUpdatedAt,
		TokensUsed:     tokensUsed,
		ContextLeftPct: contextLeftPct,
	}, nil
}

func parsePanePreview(line string) (PanePreview, error) {
	parts := strings.Split(line, "\t")
	if len(parts) != 15 {
		return PanePreview{}, fmt.Errorf("expected 15 preview columns, got %d", len(parts))
	}
	if strings.TrimSpace(parts[0]) == "" {
		return PanePreview{}, errors.New("pane_id is required")
	}
	deadValue, err := strconv.Atoi(parts[8])
	if err != nil {
		return PanePreview{}, fmt.Errorf("invalid pane_dead: %w", err)
	}
	if deadValue != 0 && deadValue != 1 {
		return PanePreview{}, fmt.Errorf("invalid pane_dead value %d", deadValue)
	}
	deadStatus := 0
	if strings.TrimSpace(parts[9]) != "" {
		deadStatus, err = strconv.Atoi(parts[9])
		if err != nil {
			return PanePreview{}, fmt.Errorf("invalid pane_dead_status: %w", err)
		}
	}
	activity, err := parseOptionalUnixTime(parts[10])
	if err != nil {
		return PanePreview{}, fmt.Errorf("invalid window_activity: %w", err)
	}
	agentUpdatedAt, err := parseOptionalUnixTime(parts[14])
	if err != nil {
		return PanePreview{}, fmt.Errorf("invalid agent_updated_at: %w", err)
	}
	return PanePreview{
		PaneID:         parts[0],
		SessionName:    parts[1],
		WindowIndex:    parts[2],
		WindowName:     parts[3],
		PaneIndex:      parts[4],
		Command:        parts[5],
		Title:          parts[6],
		Path:           parts[7],
		Dead:           deadValue == 1,
		DeadStatus:     deadStatus,
		WindowActivity: activity,
		LocalStatus:    strings.TrimSpace(parts[11]),
		AgentStatus:    strings.TrimSpace(parts[12]),
		AgentTool:      strings.TrimSpace(parts[13]),
		AgentUpdatedAt: agentUpdatedAt,
	}, nil
}

func parseOptionalInt(raw string) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func parseOptionalUnixTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	secs, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	if secs <= 0 {
		return time.Time{}, nil
	}
	return time.Unix(secs, 0).UTC(), nil
}
