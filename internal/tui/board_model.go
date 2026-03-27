package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alnah/panefleet/internal/board"
	"github.com/alnah/panefleet/internal/state"
)

// BoardRuntime defines the richer board operations required by the live Bubble
// Tea board.
type BoardRuntime interface {
	Rows(ctx context.Context) ([]board.Row, error)
	Preview(ctx context.Context, paneID string) (board.Preview, error)
	ToggleStaleOverride(ctx context.Context, paneID string) (state.PaneState, error)
	JumpToRow(ctx context.Context, row board.Row) error
	KillPane(ctx context.Context, paneID string) error
	RespawnPane(ctx context.Context, paneID string) error
	Subscribe() (<-chan state.PaneState, func())
}

type refreshPriority int

const (
	priorityStartup refreshPriority = iota
	priorityUser
	priorityBackground
)

type boardRowsMsg struct {
	rows     []board.Row
	err      error
	priority refreshPriority
}

type boardPreviewMsg struct {
	paneID  string
	preview board.Preview
	err     error
}

type boardTickMsg time.Time

type boardStateUpdatedMsg struct {
	paneID string
}

type boardActionMsg struct {
	err              error
	refreshPriority  refreshPriority
	refreshPreviewID string
}

// BoardModel drives the live board UI with decoupled table and preview
// refreshes.
type BoardModel struct {
	runtime         BoardRuntime
	interval        time.Duration
	opTimeout       time.Duration
	previewTimeout  time.Duration
	rows            []board.Row
	selectedPaneID  string
	preview         board.Preview
	width           int
	height          int
	err             error
	lastRowsRefresh time.Time
	rowsLoaded      bool
	rowsFetching    bool
	previewFetching bool
	queuedRows      *refreshPriority
	queuedPreviewID string
	updates         <-chan state.PaneState
	cancelUpdates   func()
}

// NewBoard constructs the primary Bubble Tea board model.
func NewBoard(runtime BoardRuntime, interval time.Duration) BoardModel {
	if interval <= 0 {
		interval = time.Second
	}
	updates, cancel := runtime.Subscribe()
	return BoardModel{
		runtime:        runtime,
		interval:       interval,
		opTimeout:      5 * time.Second,
		previewTimeout: 3 * time.Second,
		rowsFetching:   true,
		updates:        updates,
		cancelUpdates:  cancel,
	}
}

// Init starts the first row fetch, the periodic tick, and the update
// subscription listener.
func (m BoardModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.fetchRowsCmd(priorityStartup), boardTickCmd(m.interval)}
	if m.updates != nil {
		cmds = append(cmds, waitForBoardUpdateCmd(m.updates))
	}
	return tea.Batch(cmds...)
}

// Update applies input, refresh, and preview messages while keeping I/O in tea
// commands and preserving input responsiveness under refresh load.
func (m BoardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.cancelUpdates != nil {
				m.cancelUpdates()
				m.cancelUpdates = nil
			}
			return m, tea.Quit
		case "up", "k":
			if paneID := m.moveSelection(-1); paneID != "" {
				return m, m.requestPreviewCmd(paneID)
			}
		case "down", "j":
			if paneID := m.moveSelection(1); paneID != "" {
				return m, m.requestPreviewCmd(paneID)
			}
		case "r":
			if cmd := m.requestRowsCmd(priorityUser); cmd != nil {
				return m, cmd
			}
		case "enter":
			if row, ok := m.selectedRow(); ok {
				return m, m.jumpCmd(row)
			}
		case "ctrl+s", "s":
			if row, ok := m.selectedRow(); ok {
				return m, m.toggleStaleCmd(row.PaneID)
			}
		case "d":
			if row, ok := m.selectedRow(); ok {
				return m, m.killCmd(row.PaneID)
			}
		case "x":
			if row, ok := m.selectedRow(); ok {
				return m, m.respawnCmd(row.PaneID)
			}
		}
	case boardRowsMsg:
		m.rowsFetching = false
		m.err = msg.err
		m.rowsLoaded = true
		if msg.err == nil {
			before := m.selectedPaneID
			m.rows = msg.rows
			m.reconcileSelection()
			m.lastRowsRefresh = time.Now().UTC()
			if m.selectedPaneID != "" && m.selectedPaneID != before {
				if cmd := m.requestPreviewCmd(m.selectedPaneID); cmd != nil {
					return m, cmd
				}
			}
			if m.preview.PaneID == "" && m.selectedPaneID != "" {
				if cmd := m.requestPreviewCmd(m.selectedPaneID); cmd != nil {
					return m, cmd
				}
			}
		}
		if m.queuedRows != nil {
			priority := *m.queuedRows
			m.queuedRows = nil
			m.rowsFetching = true
			return m, m.fetchRowsCmd(priority)
		}
	case boardPreviewMsg:
		m.previewFetching = false
		if msg.err != nil {
			m.err = msg.err
		} else if msg.paneID == m.selectedPaneID {
			m.preview = msg.preview
		}
		if m.queuedPreviewID != "" {
			next := m.queuedPreviewID
			m.queuedPreviewID = ""
			m.previewFetching = true
			return m, m.fetchPreviewCmd(next)
		}
	case boardTickMsg:
		cmds := []tea.Cmd{boardTickCmd(m.interval)}
		if cmd := m.requestRowsCmd(priorityBackground); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.requestPreviewCmd(m.selectedPaneID); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	case boardStateUpdatedMsg:
		cmds := make([]tea.Cmd, 0, 2)
		if msg.paneID != "" && msg.paneID == m.selectedPaneID {
			if cmd := m.requestPreviewCmd(msg.paneID); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if cmd := m.requestRowsCmd(priorityUser); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if m.updates != nil {
			cmds = append(cmds, waitForBoardUpdateCmd(m.updates))
		}
		return m, tea.Batch(cmds...)
	case boardActionMsg:
		m.err = msg.err
		cmds := make([]tea.Cmd, 0, 2)
		if msg.refreshPreviewID != "" {
			if cmd := m.requestPreviewCmd(msg.refreshPreviewID); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if cmd := m.requestRowsCmd(msg.refreshPriority); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// View renders the board across the full alt-screen area, with a table on top
// and preview below.
func (m BoardModel) View() string {
	width := m.width
	if width <= 0 {
		width = 120
	}
	height := m.height
	if height <= 0 {
		height = 30
	}

	lines := make([]string, 0, height)
	lines = append(lines, fitLine("Panefleet Board", width))
	lines = append(lines, fitLine("enter: jump  j/k,up/down: move  ctrl+s: stale  x: respawn  d: kill  r: refresh  q: quit", width))
	if m.err != nil {
		lines = append(lines, fitLine("error: "+m.err.Error(), width))
	} else if !m.rowsLoaded {
		lines = append(lines, fitLine("loading panes...", width))
	} else {
		lines = append(lines, fitLine(fmt.Sprintf("rows: %d  last refresh: %s", len(m.rows), formatRefresh(m.lastRowsRefresh)), width))
	}
	lines = append(lines, fitLine(strings.Repeat("─", width), width))

	topHeight := max(8, height/2)
	tableLines := m.renderTable(width, max(3, topHeight-len(lines)-1))
	lines = append(lines, tableLines...)
	lines = append(lines, fitLine(strings.Repeat("─", width), width))
	previewLines := m.renderPreview(width, height-len(lines))
	lines = append(lines, previewLines...)

	if len(lines) < height {
		for len(lines) < height {
			lines = append(lines, "")
		}
	} else if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func (m *BoardModel) requestRowsCmd(priority refreshPriority) tea.Cmd {
	if m.rowsFetching {
		m.queueRows(priority)
		return nil
	}
	m.rowsFetching = true
	return m.fetchRowsCmd(priority)
}

func (m *BoardModel) queueRows(priority refreshPriority) {
	if m.queuedRows == nil || priority < *m.queuedRows {
		p := priority
		m.queuedRows = &p
	}
}

func (m *BoardModel) requestPreviewCmd(paneID string) tea.Cmd {
	if paneID == "" {
		return nil
	}
	if m.previewFetching {
		m.queuedPreviewID = paneID
		return nil
	}
	m.previewFetching = true
	return m.fetchPreviewCmd(paneID)
}

func (m *BoardModel) fetchRowsCmd(priority refreshPriority) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), m.opTimeout)
		defer cancel()

		rows, err := m.runtime.Rows(ctx)
		return boardRowsMsg{rows: rows, err: err, priority: priority}
	}
}

func (m *BoardModel) fetchPreviewCmd(paneID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), m.previewTimeout)
		defer cancel()

		preview, err := m.runtime.Preview(ctx, paneID)
		return boardPreviewMsg{paneID: paneID, preview: preview, err: err}
	}
}

func (m BoardModel) toggleStaleCmd(paneID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), m.opTimeout)
		defer cancel()

		_, err := m.runtime.ToggleStaleOverride(ctx, paneID)
		return boardActionMsg{
			err:              err,
			refreshPriority:  priorityUser,
			refreshPreviewID: paneID,
		}
	}
}

func (m BoardModel) jumpCmd(row board.Row) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), m.opTimeout)
		defer cancel()

		return boardActionMsg{
			err:             m.runtime.JumpToRow(ctx, row),
			refreshPriority: priorityBackground,
		}
	}
}

func (m BoardModel) killCmd(paneID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), m.opTimeout)
		defer cancel()

		return boardActionMsg{
			err:              m.runtime.KillPane(ctx, paneID),
			refreshPriority:  priorityUser,
			refreshPreviewID: paneID,
		}
	}
}

func (m BoardModel) respawnCmd(paneID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), m.opTimeout)
		defer cancel()

		return boardActionMsg{
			err:              m.runtime.RespawnPane(ctx, paneID),
			refreshPriority:  priorityUser,
			refreshPreviewID: paneID,
		}
	}
}

func boardTickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg { return boardTickMsg(t) })
}

func waitForBoardUpdateCmd(ch <-chan state.PaneState) tea.Cmd {
	return func() tea.Msg {
		st, ok := <-ch
		if !ok {
			return nil
		}
		return boardStateUpdatedMsg{paneID: st.PaneID}
	}
}

func (m *BoardModel) reconcileSelection() {
	if len(m.rows) == 0 {
		m.selectedPaneID = ""
		return
	}
	for _, row := range m.rows {
		if row.PaneID == m.selectedPaneID {
			return
		}
	}
	m.selectedPaneID = m.rows[0].PaneID
}

func (m *BoardModel) moveSelection(delta int) string {
	if len(m.rows) == 0 {
		return ""
	}
	index := m.selectedIndex()
	next := index + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.rows) {
		next = len(m.rows) - 1
	}
	if next == index {
		return ""
	}
	m.selectedPaneID = m.rows[next].PaneID
	return m.selectedPaneID
}

func (m BoardModel) selectedIndex() int {
	for i, row := range m.rows {
		if row.PaneID == m.selectedPaneID {
			return i
		}
	}
	return 0
}

func (m BoardModel) selectedRow() (board.Row, bool) {
	if len(m.rows) == 0 {
		return board.Row{}, false
	}
	for _, row := range m.rows {
		if row.PaneID == m.selectedPaneID {
			return row, true
		}
	}
	return m.rows[0], true
}

func (m BoardModel) renderTable(width, height int) []string {
	lines := []string{fitLine(renderTableHeader(width), width)}
	if !m.rowsLoaded {
		lines = append(lines, fitLine("loading panes...", width))
		return padLines(lines, height)
	}
	if len(m.rows) == 0 {
		lines = append(lines, fitLine("(no panes)", width))
		return padLines(lines, height)
	}

	selected := m.selectedIndex()
	start := 0
	if selected > height-3 {
		start = selected - (height - 3)
	}
	end := min(len(m.rows), start+max(1, height-1))
	for i := start; i < end; i++ {
		lines = append(lines, fitLine(renderTableRow(m.rows[i], i == selected, width), width))
	}
	return padLines(lines, height)
}

func renderTableHeader(width int) string {
	status, tool, target, session, window, repo, tokens, ctx := boardLayoutWidths(width)
	return joinColumns(
		fitCell("STATE", status),
		fitCell("TOOL", tool),
		fitCell("TARGET", target),
		fitCell("SESSION", session),
		fitCell("WINDOW", window),
		fitCell("REPO", repo),
		fitCell("TOKENS", tokens),
		fitCell("CTX%", ctx),
		"AGE",
	)
}

func renderTableRow(row board.Row, selected bool, width int) string {
	status, tool, target, session, window, repo, tokens, ctx := boardLayoutWidths(width)
	marker := " "
	if selected {
		marker = ">"
	}
	return marker + " " + joinColumns(
		fitCell(string(row.Status), status),
		fitCell(row.Tool, tool),
		fitCell(row.TargetPane(), target),
		fitCell(row.SessionName, session),
		fitCell(row.WindowName, window),
		fitCell(row.Repo, repo),
		fitCell(optionalInt(row.TokensUsed), tokens),
		fitCell(optionalPercent(row.ContextLeftPct), ctx),
		prettyAge(row.WindowActivity),
	)
}

func (m BoardModel) renderPreview(width, height int) []string {
	lines := make([]string, 0, height)
	if !m.rowsLoaded {
		return padLines([]string{"loading preview..."}, height)
	}
	if m.selectedPaneID == "" {
		return padLines([]string{"(no preview)"}, height)
	}
	if m.preview.PaneID == "" || m.preview.PaneID != m.selectedPaneID {
		return padLines([]string{fmt.Sprintf("loading preview for %s", m.selectedPaneID)}, height)
	}
	lines = append(lines, fitLine(fmt.Sprintf("status: %s  tool: %s  target: %s:%s.%s", m.preview.Status, m.preview.Tool, m.preview.SessionName, m.preview.WindowIndex, m.preview.PaneIndex), width))
	lines = append(lines, fitLine(fmt.Sprintf("window: %s  cmd: %s  title: %s", m.preview.WindowName, m.preview.Command, m.preview.Title), width))
	lines = append(lines, fitLine(fmt.Sprintf("path: %s", m.preview.Path), width))
	lines = append(lines, fitLine(strings.Repeat("─", width), width))
	bodyLines := strings.Split(m.preview.Body, "\n")
	remaining := max(0, height-len(lines))
	for _, line := range bodyLines {
		if remaining == 0 {
			break
		}
		lines = append(lines, fitLine(line, width))
		remaining--
	}
	return padLines(lines, height)
}

func boardLayoutWidths(width int) (status, tool, target, session, window, repo, tokens, ctx int) {
	if width < 80 {
		width = 80
	}
	status = 6
	tool = 8
	target = 8
	tokens = 10
	ctx = 5
	fixed := status + tool + target + tokens + ctx + len(" │ ")*7 + 6
	flex := max(24, width-fixed)
	session = max(8, flex*25/100)
	window = max(12, flex*35/100)
	repo = max(8, flex-session-window)
	return
}

func fitCell(text string, width int) string {
	if width <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) > width {
		if width <= 1 {
			return string(runes[:width])
		}
		return string(runes[:width-1]) + "…"
	}
	return text + strings.Repeat(" ", width-len(runes))
}

func fitLine(text string, width int) string {
	return fitCell(text, width)
}

func joinColumns(parts ...string) string {
	return strings.Join(parts, " │ ")
}

func padLines(lines []string, height int) []string {
	if height <= 0 {
		return lines
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		return lines[:height]
	}
	return lines
}

func optionalInt(value *int) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *value)
}

func optionalPercent(value *int) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%d%%", *value)
}

func prettyAge(at time.Time) string {
	if at.IsZero() {
		return "-"
	}
	diff := time.Since(at)
	switch {
	case diff < time.Minute:
		return fmt.Sprintf("%ds", int(diff.Seconds()))
	case diff < time.Hour:
		return fmt.Sprintf("%dm", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh", int(diff.Hours()))
	default:
		return fmt.Sprintf("%dd", int(diff.Hours()/24))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
