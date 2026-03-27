package board

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/alnah/panefleet/internal/state"
	"github.com/alnah/panefleet/internal/tmuxctl"
)

// StateSource defines the lifecycle operations needed by the board runtime.
type StateSource interface {
	StateShow(ctx context.Context, paneID string) (state.PaneState, error)
	StateList(ctx context.Context) ([]state.PaneState, error)
	SetOverride(ctx context.Context, paneID string, target state.Status, source string) (state.PaneState, error)
	ClearOverride(ctx context.Context, paneID, source string) (state.PaneState, error)
	Subscribe() (<-chan state.PaneState, func())
}

// TMUX defines the tmux operations needed by the board runtime.
type TMUX interface {
	BoardSnapshot(ctx context.Context) ([]tmuxctl.BoardPane, error)
	Preview(ctx context.Context, paneID string) (tmuxctl.PanePreview, error)
	RecentCapture(ctx context.Context, paneID string) (string, error)
	JumpToPane(ctx context.Context, paneID, targetWindow string) error
	KillPane(ctx context.Context, paneID string) error
	RespawnPane(ctx context.Context, paneID string) error
}

// Row is the stable read-model entry rendered by the board table.
type Row struct {
	PaneID           string
	Status           state.Status
	Tool             string
	SessionName      string
	WindowIndex      string
	WindowName       string
	PaneIndex        string
	Path             string
	Repo             string
	Command          string
	Title            string
	TokensUsed       *int
	ContextLeftPct   *int
	WindowActivity   time.Time
	StatusSource     string
	ReasonCode       string
	ManualOverride   *state.Status
	LastTransitionAt time.Time
}

// TargetPane returns the table target identifier used by the shell board.
func (r Row) TargetPane() string {
	return fmt.Sprintf("%s.%s", r.WindowIndex, r.PaneIndex)
}

// TargetWindow returns the tmux target used by jump operations.
func (r Row) TargetWindow() string {
	return fmt.Sprintf("%s:%s", r.SessionName, r.WindowIndex)
}

// Preview is the data rendered in the bottom pane for the selected row.
type Preview struct {
	PaneID      string
	Status      state.Status
	Tool        string
	SessionName string
	WindowIndex string
	WindowName  string
	PaneIndex   string
	Command     string
	Title       string
	Path        string
	Body        string
	LoadedAt    time.Time
}

// Service assembles board rows and preview data from the existing lifecycle
// and tmux boundaries.
type Service struct {
	states        StateSource
	tmux          TMUX
	currentPaneID string
	codexMetrics  *codexMetricsResolver
	claudeMetrics *claudeMetricsResolver
	openMetrics   *openCodeMetricsResolver
	now           func() time.Time
}

const boardStaleAfter = 45 * time.Minute
const boardDoneRecentWindow = 10 * time.Minute
const boardAgentStatusMaxAge = 10 * time.Minute

// NewService builds the board read-model boundary.
func NewService(states StateSource, tmux TMUX, currentPaneID string) *Service {
	return &Service{
		states:        states,
		tmux:          tmux,
		currentPaneID: strings.TrimSpace(currentPaneID),
		codexMetrics:  newCodexMetricsResolver(),
		claudeMetrics: newClaudeMetricsResolver(),
		openMetrics:   newOpenCodeMetricsResolver(),
		now:           func() time.Time { return time.Now().UTC() },
	}
}

// Subscribe forwards pane-state updates so the UI can schedule refreshes
// without depending on the underlying lifecycle implementation.
func (s *Service) Subscribe() (<-chan state.PaneState, func()) {
	return s.states.Subscribe()
}

// Rows joins tmux metadata with canonical pane state and returns a stable,
// sortable board snapshot.
func (s *Service) Rows(ctx context.Context) ([]Row, error) {
	snapshot, err := s.tmux.BoardSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("board rows snapshot: %w", err)
	}
	states, err := s.states.StateList(ctx)
	if err != nil {
		return nil, fmt.Errorf("board rows state-list: %w", err)
	}

	byPane := make(map[string]state.PaneState, len(states))
	for _, st := range states {
		byPane[st.PaneID] = st
	}
	now := s.now()

	rows := make([]Row, 0, len(snapshot))
	for _, pane := range snapshot {
		if s.currentPaneID != "" && pane.PaneID == s.currentPaneID {
			continue
		}
		st, ok := byPane[pane.PaneID]
		if !ok {
			st = state.NewPaneState(pane.PaneID).Effective()
		}
		repo := repoName(pane.Path)
		tool := toolKind(pane.Command, pane.WindowName, pane.Title)
		tokensUsed, contextLeftPct := s.resolveRowMetrics(ctx, pane, tool)
		status := s.resolveStatus(ctx, st, pane, now)
		rows = append(rows, Row{
			PaneID:           pane.PaneID,
			Status:           status,
			Tool:             tool,
			SessionName:      pane.SessionName,
			WindowIndex:      pane.WindowIndex,
			WindowName:       pane.WindowName,
			PaneIndex:        pane.PaneIndex,
			Path:             pane.Path,
			Repo:             repo,
			Command:          pane.Command,
			Title:            pane.Title,
			TokensUsed:       tokensUsed,
			ContextLeftPct:   contextLeftPct,
			WindowActivity:   pane.WindowActivity,
			StatusSource:     st.StatusSource,
			ReasonCode:       st.ReasonCode,
			ManualOverride:   copyStatusPtr(st.ManualOverride),
			LastTransitionAt: st.LastTransitionAt,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		pi := priority(rows[i].Status)
		pj := priority(rows[j].Status)
		if pi != pj {
			return pi < pj
		}
		if !rows[i].WindowActivity.Equal(rows[j].WindowActivity) {
			return rows[i].WindowActivity.After(rows[j].WindowActivity)
		}
		return rows[i].PaneID < rows[j].PaneID
	})

	return rows, nil
}

func (s *Service) resolveRowMetrics(ctx context.Context, pane tmuxctl.BoardPane, tool string) (*int, *int) {
	tokensUsed := pane.TokensUsed
	contextLeftPct := pane.ContextLeftPct
	if tokensUsed != nil && contextLeftPct != nil {
		return tokensUsed, contextLeftPct
	}

	var (
		metrics codexMetrics
		ok      bool
		err     error
	)
	switch tool {
	case "codex":
		if s.codexMetrics == nil {
			return tokensUsed, contextLeftPct
		}
		metrics, ok, err = s.codexMetrics.resolve(ctx, pane)
	case "claude":
		if s.claudeMetrics == nil {
			return tokensUsed, contextLeftPct
		}
		metrics, ok, err = s.claudeMetrics.resolve(ctx, pane)
	case "opencode":
		if s.openMetrics == nil {
			return tokensUsed, contextLeftPct
		}
		metrics, ok, err = s.openMetrics.resolve(ctx, pane)
	default:
		return tokensUsed, contextLeftPct
	}
	if err != nil || !ok {
		return tokensUsed, contextLeftPct
	}
	if tokensUsed == nil {
		tokensUsed = metrics.TokensUsed
	}
	if contextLeftPct == nil {
		contextLeftPct = metrics.ContextLeftPct
	}
	return tokensUsed, contextLeftPct
}

// Preview loads live metadata and body capture for one pane.
func (s *Service) Preview(ctx context.Context, paneID string) (Preview, error) {
	if s.currentPaneID != "" && paneID == s.currentPaneID {
		return Preview{}, fmt.Errorf("board preview pane=%s: current board pane is hidden", paneID)
	}
	pane, err := s.tmux.Preview(ctx, paneID)
	if err != nil {
		return Preview{}, fmt.Errorf("board preview pane=%s: %w", paneID, err)
	}
	st, err := s.states.StateShow(ctx, paneID)
	if err != nil {
		if !isPaneNotFound(err) {
			return Preview{}, fmt.Errorf("board preview state pane=%s: %w", paneID, err)
		}
		st = state.NewPaneState(paneID).Effective()
	}
	boardPane := tmuxctl.BoardPane{
		PaneID:         pane.PaneID,
		SessionName:    pane.SessionName,
		WindowIndex:    pane.WindowIndex,
		WindowName:     pane.WindowName,
		PaneIndex:      pane.PaneIndex,
		Command:        pane.Command,
		Title:          pane.Title,
		Path:           pane.Path,
		Dead:           pane.Dead,
		DeadStatus:     pane.DeadStatus,
		WindowActivity: pane.WindowActivity,
		LocalStatus:    pane.LocalStatus,
		AgentStatus:    pane.AgentStatus,
		AgentTool:      pane.AgentTool,
		AgentUpdatedAt: pane.AgentUpdatedAt,
	}
	status := s.resolveStatus(ctx, st, boardPane, s.now())

	return Preview{
		PaneID:      pane.PaneID,
		Status:      status,
		Tool:        toolKind(pane.Command, pane.WindowName, pane.Title),
		SessionName: pane.SessionName,
		WindowIndex: pane.WindowIndex,
		WindowName:  pane.WindowName,
		PaneIndex:   pane.PaneIndex,
		Command:     pane.Command,
		Title:       pane.Title,
		Path:        pane.Path,
		Body:        pane.Body,
		LoadedAt:    s.now(),
	}, nil
}

// ToggleStaleOverride toggles the stale manual override on the selected pane.
func (s *Service) ToggleStaleOverride(ctx context.Context, paneID string) (state.PaneState, error) {
	current, err := s.states.StateShow(ctx, paneID)
	if err != nil {
		return state.PaneState{}, fmt.Errorf("toggle stale pane=%s: %w", paneID, err)
	}
	if current.ManualOverride != nil && *current.ManualOverride == state.StatusStale {
		return s.states.ClearOverride(ctx, paneID, "board")
	}
	return s.states.SetOverride(ctx, paneID, state.StatusStale, "board")
}

// JumpToRow focuses the tmux pane represented by the selected row.
func (s *Service) JumpToRow(ctx context.Context, row Row) error {
	return s.tmux.JumpToPane(ctx, row.PaneID, row.TargetWindow())
}

// KillPane forwards the kill operation to tmux.
func (s *Service) KillPane(ctx context.Context, paneID string) error {
	return s.tmux.KillPane(ctx, paneID)
}

// RespawnPane forwards the respawn operation to tmux.
func (s *Service) RespawnPane(ctx context.Context, paneID string) error {
	return s.tmux.RespawnPane(ctx, paneID)
}

func repoName(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	base := filepath.Base(strings.TrimRight(trimmed, "/"))
	if base == "." || base == "/" {
		return ""
	}
	return base
}

func toolKind(command, windowName, title string) string {
	cmd := strings.ToLower(strings.TrimSpace(command))
	window := strings.ToLower(strings.TrimSpace(windowName))
	name := strings.ToLower(strings.TrimSpace(title))

	switch {
	case strings.HasPrefix(cmd, "codex"):
		return "codex"
	case strings.HasPrefix(cmd, "claude"), strings.Contains(window, "claude"), strings.Contains(name, "claude"):
		return "claude"
	case cmd == "opencode", cmd == "open-code", strings.Contains(window, "opencode"), strings.Contains(name, "opencode"):
		return "opencode"
	case cmd == "sh", cmd == "bash", cmd == "zsh", cmd == "fish", cmd == "nu", cmd == "tmux":
		return "shell"
	case cmd == "":
		return "unknown"
	default:
		return command
	}
}

func copyStatusPtr(in *state.Status) *state.Status {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func priority(st state.Status) int {
	switch st {
	case state.StatusWait:
		return 0
	case state.StatusRun:
		return 1
	case state.StatusDone:
		return 2
	case state.StatusError:
		return 3
	case state.StatusIdle:
		return 4
	case state.StatusStale:
		return 5
	default:
		return 6
	}
}

func displayStatus(st state.PaneState, pane tmuxctl.BoardPane, now time.Time) state.Status {
	if status, ok := parseExplicitStatus(pane.LocalStatus); ok {
		return effectiveStatus(status, pane.WindowActivity, now)
	}
	if pane.Dead {
		if pane.DeadStatus == 0 {
			return state.StatusDone
		}
		return state.StatusError
	}
	if status, ok := parseAgentStatus(pane, now); ok {
		return effectiveStatus(status, pane.WindowActivity, now)
	}
	if st.Status != state.StatusUnknown {
		return st.Status
	}
	if pane.WindowActivity.IsZero() {
		return state.StatusIdle
	}
	if now.Sub(pane.WindowActivity) >= boardStaleAfter {
		return state.StatusStale
	}
	return state.StatusIdle
}

func (s *Service) resolveStatus(ctx context.Context, st state.PaneState, pane tmuxctl.BoardPane, now time.Time) state.Status {
	if status, ok := parseExplicitStatus(pane.LocalStatus); ok {
		return effectiveStatus(status, pane.WindowActivity, now)
	}
	if pane.Dead {
		if pane.DeadStatus == 0 {
			return state.StatusDone
		}
		return state.StatusError
	}

	heuristic, heuristicOK := s.captureStatus(ctx, pane, now)
	if status, ok := parseAgentStatus(pane, now); ok {
		status = effectiveStatus(status, pane.WindowActivity, now)
		if override, ok := overrideFreshAgentStatus(toolKind(pane.Command, pane.WindowName, pane.Title), status, heuristic, heuristicOK); ok {
			return override
		}
		return status
	}
	if override, ok := overrideStoredStatus(st.Status, heuristic, heuristicOK); ok {
		return override
	}
	if st.Status != state.StatusUnknown {
		return st.Status
	}
	if heuristicOK {
		return heuristic
	}
	return idleOrStale(pane.WindowActivity, now)
}

func parseExplicitStatus(raw string) (state.Status, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	status, err := state.ParseStatus(raw)
	if err != nil {
		return "", false
	}
	return status, true
}

func parseAgentStatus(pane tmuxctl.BoardPane, now time.Time) (state.Status, bool) {
	status, ok := parseExplicitStatus(pane.AgentStatus)
	if !ok {
		return "", false
	}
	if !pane.AgentUpdatedAt.IsZero() && now.Sub(pane.AgentUpdatedAt) > boardAgentStatusMaxAge {
		return "", false
	}
	liveTool := toolKind(pane.Command, pane.WindowName, pane.Title)
	if toolKind(strings.TrimSpace(pane.AgentTool), "", "") != liveTool && strings.TrimSpace(pane.AgentTool) != "" {
		return "", false
	}
	return status, true
}

func effectiveStatus(raw state.Status, activity, now time.Time) state.Status {
	switch raw {
	case state.StatusError, state.StatusWait, state.StatusRun:
		return raw
	case state.StatusDone:
		if !activity.IsZero() && now.Sub(activity) < boardDoneRecentWindow {
			return state.StatusDone
		}
		return idleOrStale(activity, now)
	case state.StatusIdle:
		return idleOrStale(activity, now)
	default:
		return raw
	}
}

func idleOrStale(activity, now time.Time) state.Status {
	if activity.IsZero() {
		return state.StatusIdle
	}
	if now.Sub(activity) >= boardStaleAfter {
		return state.StatusStale
	}
	return state.StatusIdle
}

func overrideFreshAgentStatus(tool string, agent state.Status, heuristic state.Status, heuristicOK bool) (state.Status, bool) {
	if !heuristicOK {
		return "", false
	}
	switch tool {
	case "codex":
		if agent != state.StatusWait && (heuristic == state.StatusRun || heuristic == state.StatusWait) {
			return heuristic, true
		}
	case "claude":
		if agent != state.StatusWait && heuristic == state.StatusWait {
			return heuristic, true
		}
	case "opencode":
		switch agent {
		case state.StatusRun:
			if heuristic == state.StatusWait || heuristic == state.StatusDone || heuristic == state.StatusIdle || heuristic == state.StatusError {
				return heuristic, true
			}
		case state.StatusWait:
			if heuristic == state.StatusRun || heuristic == state.StatusDone || heuristic == state.StatusIdle || heuristic == state.StatusError {
				return heuristic, true
			}
		}
	}
	return "", false
}

func overrideStoredStatus(stored state.Status, heuristic state.Status, heuristicOK bool) (state.Status, bool) {
	if !heuristicOK {
		return "", false
	}
	switch stored {
	case state.StatusUnknown:
		return "", false
	case state.StatusIdle, state.StatusStale:
		return heuristic, true
	case state.StatusDone:
		if heuristic == state.StatusRun || heuristic == state.StatusWait || heuristic == state.StatusError {
			return heuristic, true
		}
	}
	return "", false
}

func (s *Service) captureStatus(ctx context.Context, pane tmuxctl.BoardPane, now time.Time) (state.Status, bool) {
	tool := toolKind(pane.Command, pane.WindowName, pane.Title)
	if tool != "codex" && tool != "claude" && tool != "opencode" {
		return "", false
	}
	capture, err := s.tmux.RecentCapture(ctx, pane.PaneID)
	if err != nil {
		return "", false
	}
	capture = strings.TrimSpace(capture)
	if capture == "" {
		return "", false
	}
	recent := tailLines(capture, 20)
	focus := tailLines(recent, 12)
	switch tool {
	case "codex":
		if captureContains(recent, "Enter to confirm", "Esc to cancel", "waiting on approval", "approval required", "select model", "choose model", "model to change", "permission mode", "approval mode", "/permissions", "/models") {
			return state.StatusWait, true
		}
		if captureContains(recent, "permission denied", "access denied", "fatal:", "traceback", "exception", "command failed", "request failed") {
			return state.StatusError, true
		}
		if captureContains(recent, "working (", "esc to interrupt", "background terminal running", "background terminals running") {
			return state.StatusRun, true
		}
		if matchAnyLine(recent, `^\s*›\s`) {
			return effectiveStatus(state.StatusDone, pane.WindowActivity, now), true
		}
		return state.StatusIdle, true
	case "claude":
		if captureContains(focus, "Enter to confirm", "Esc to cancel", "Do you want to", "approval required", "waiting on approval", "choose an option", "select an option", "Yes, proceed", "No, cancel", "/permissions", "Allow Ask Deny", "allow all edits", "allow once", "deny once", "deny all") || matchAnyLine(focus, `(?i)press .*navigate`) {
			return state.StatusWait, true
		}
		if captureContains(focus, "permission denied", "access denied", "fatal:", "traceback", "exception", "command failed", "tool failed") {
			return state.StatusError, true
		}
		if matchAnyLine(focus, `^\s*❯\s*$`) {
			return effectiveStatus(state.StatusDone, pane.WindowActivity, now), true
		}
		if captureContains(focus, "⏺ ", "Bash(", "Read(", "Write(", "Edit(", "MultiEdit(", "Glob(", "Grep(", "LS(", "Task(", "Running", "Processing") {
			return state.StatusRun, true
		}
		return state.StatusIdle, true
	case "opencode":
		if captureContains(focus, "Enter to confirm", "Esc to cancel", "approval", "permission", "confirm", "cancel", "select model", "select variant") {
			return state.StatusWait, true
		}
		if captureContains(focus, "Ask anything...", "ctrl+p commands", "tab agents") && !captureContains(focus, "Thinking:", "Preparing patch", "Applying patch", "Running", "Generating", "Writing", "Tool execution", "esc interrupt") {
			return effectiveStatus(state.StatusDone, pane.WindowActivity, now), true
		}
		if captureContains(focus, "permission denied", "access denied", "fatal:", "traceback", "exception", "command failed", "request failed") {
			return state.StatusError, true
		}
		if captureContains(recent, "Thinking:", "Preparing patch", "Applying patch", "Running", "Generating", "Writing", "Tool execution", "esc interrupt") {
			return state.StatusRun, true
		}
		return state.StatusIdle, true
	}
	return "", false
}

func captureContains(capture string, needles ...string) bool {
	lower := strings.ToLower(capture)
	for _, needle := range needles {
		if strings.Contains(lower, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func tailLines(input string, maxLines int) string {
	if maxLines <= 0 {
		return input
	}
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}

func matchAnyLine(input, pattern string) bool {
	re := regexp.MustCompile(pattern)
	for _, line := range strings.Split(input, "\n") {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

func isPaneNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "pane not found") || strings.TrimSpace(message) == "not found"
}
