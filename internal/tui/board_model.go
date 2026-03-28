package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
	paneState        *state.PaneState
	quitOnSuccess    bool
	refreshPriority  refreshPriority
	refreshPreviewID string
}

// BoardModel drives the live board UI with decoupled table and preview
// refreshes.
type BoardModel struct {
	runtime         BoardRuntime
	styles          boardStyles
	interval        time.Duration
	opTimeout       time.Duration
	previewTimeout  time.Duration
	rows            []board.Row
	selectedPaneID  string
	preview         board.Preview
	searchQuery     string
	searchActive    bool
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
func NewBoard(runtime BoardRuntime, interval time.Duration, themeName string) BoardModel {
	if interval <= 0 {
		interval = time.Second
	}
	updates, cancel := runtime.Subscribe()
	return BoardModel{
		runtime:        runtime,
		styles:         newBoardStyles(themeName),
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
		if handled, cmd := m.handleSearchKey(msg); handled {
			return m, cmd
		}
		switch msg.String() {
		case "esc", "ctrl+c":
			if m.cancelUpdates != nil {
				m.cancelUpdates()
				m.cancelUpdates = nil
			}
			return m, tea.Quit
		case "up":
			if paneID := m.moveSelection(-1); paneID != "" {
				return m, m.requestPreviewCmd(paneID)
			}
		case "down":
			if paneID := m.moveSelection(1); paneID != "" {
				return m, m.requestPreviewCmd(paneID)
			}
		case "enter":
			if row, ok := m.selectedRow(); ok {
				return m, m.jumpCmd(row)
			}
		case "ctrl+s":
			if row, ok := m.selectedRow(); ok {
				return m, m.toggleStaleCmd(row.PaneID)
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
		if msg.err == nil && msg.quitOnSuccess {
			if m.cancelUpdates != nil {
				m.cancelUpdates()
				m.cancelUpdates = nil
			}
			return m, tea.Quit
		}
		if msg.err == nil && msg.paneState != nil {
			m.applyPaneState(*msg.paneState)
		}
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

// View renders the board across the full alt-screen area with a full-width
// header and a main content area that prefers a horizontal board/preview split.
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
	lines = append(lines, m.renderHeader(width)...)
	lines = append(lines, m.renderMainContent(width, max(0, height-len(lines)))...)

	if len(lines) < height {
		for len(lines) < height {
			lines = append(lines, "")
		}
	} else if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func (m BoardModel) renderMainContent(width, height int) []string {
	if height <= 0 {
		return nil
	}
	leftWidth, rightWidth, horizontal := boardPanelWidths(width)
	if horizontal {
		return m.renderHorizontalContent(leftWidth, rightWidth, height)
	}
	return m.renderVerticalContent(width, height)
}

func (m BoardModel) renderHorizontalContent(leftWidth, rightWidth, height int) []string {
	separator := m.styles.separator.Render(" │ ")
	lines := make([]string, 0, height)
	leftLines := append([]string{fitLine(m.renderSectionBar("board", ""), leftWidth)}, m.renderTable(leftWidth, max(0, height-1))...)
	rightLines := append([]string{fitLine(m.renderSectionBar("preview", ""), rightWidth)}, m.renderPreview(rightWidth, max(0, height-1))...)

	for i := 0; i < height; i++ {
		leftLine := fitLine(leftLines[i], leftWidth)
		rightLine := fitLine(rightLines[i], rightWidth)
		lines = append(lines, leftLine+separator+rightLine)
	}
	return lines
}

func (m BoardModel) renderVerticalContent(width, height int) []string {
	lines := make([]string, 0, height)
	tableSectionHeight := max(4, height/2)
	tableLines := max(1, tableSectionHeight-1)
	if tableLines > max(1, height-2) {
		tableLines = max(1, height-2)
	}

	lines = append(lines, fitLine(m.renderSectionBar("board", ""), width))
	lines = append(lines, m.renderTable(width, tableLines)...)

	remaining := height - len(lines)
	if remaining <= 0 {
		return lines[:height]
	}

	lines = append(lines, fitLine(m.renderSectionBar("preview", ""), width))
	lines = append(lines, m.renderPreview(width, max(0, height-len(lines)))...)
	return lines
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

		paneState, err := m.runtime.ToggleStaleOverride(ctx, paneID)
		return boardActionMsg{
			err:              err,
			paneState:        &paneState,
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
			err:           m.runtime.JumpToRow(ctx, row),
			quitOnSuccess: true,
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
	rows := m.filteredRows()
	if len(rows) == 0 {
		m.selectedPaneID = ""
		return
	}
	for _, row := range rows {
		if row.PaneID == m.selectedPaneID {
			return
		}
	}
	m.selectedPaneID = rows[0].PaneID
}

func (m *BoardModel) moveSelection(delta int) string {
	rows := m.filteredRows()
	if len(rows) == 0 {
		return ""
	}
	index := m.selectedIndex()
	next := index + delta
	if next < 0 {
		next = 0
	}
	if next >= len(rows) {
		next = len(rows) - 1
	}
	if next == index {
		return ""
	}
	m.selectedPaneID = rows[next].PaneID
	return m.selectedPaneID
}

func (m BoardModel) selectedIndex() int {
	for i, row := range m.filteredRows() {
		if row.PaneID == m.selectedPaneID {
			return i
		}
	}
	return 0
}

func (m BoardModel) selectedRow() (board.Row, bool) {
	rows := m.filteredRows()
	if len(rows) == 0 {
		return board.Row{}, false
	}
	for _, row := range rows {
		if row.PaneID == m.selectedPaneID {
			return row, true
		}
	}
	return rows[0], true
}

func (m BoardModel) renderTable(width, height int) []string {
	lines := []string{fitLine(m.renderTableHeader(width), width)}
	if !m.rowsLoaded {
		lines = append(lines, fitLine(m.styles.info.Render("loading panes..."), width))
		return padLines(lines, height)
	}
	rows := m.filteredRows()
	if len(rows) == 0 {
		if strings.TrimSpace(m.searchQuery) != "" {
			lines = append(lines, fitLine(m.styles.info.Render("(no matches)"), width))
			return padLines(lines, height)
		}
		lines = append(lines, fitLine(m.styles.info.Render("(no panes)"), width))
		return padLines(lines, height)
	}

	selected := m.selectedIndex()
	start := 0
	if selected > height-3 {
		start = selected - (height - 3)
	}
	end := min(len(rows), start+max(1, height-1))
	for i := start; i < end; i++ {
		lines = append(lines, m.renderTableRow(rows[i], i == selected, width))
	}
	return padLines(lines, height)
}

func (m BoardModel) renderTableHeader(width int) string {
	status, tool, target, session, window, repo, tokens, ctx := boardLayoutWidths(width)
	return "  " + joinColumns(
		m.styles.headerCell.Render(fitCell("STATE", status)),
		m.styles.headerCell.Render(fitCell("TOOL", tool)),
		m.styles.headerCell.Render(fitCell("TARGET", target)),
		m.styles.headerCell.Render(fitCell("SESSION", session)),
		m.styles.headerCell.Render(fitCell("WINDOW", window)),
		m.styles.headerCell.Render(fitCell("REPO", repo)),
		m.styles.headerCell.Render(fitCell("TOKENS", tokens)),
		m.styles.headerCell.Render(fitCell("CTX%", ctx)),
		m.styles.headerCell.Render("AGE"),
	)
}

func (m BoardModel) renderTableRow(row board.Row, selected bool, width int) string {
	status, tool, target, session, window, repo, tokens, ctx := boardLayoutWidths(width)
	marker := " "
	markerStyle := m.styles.rowMarker
	if selected {
		marker = "┃"
		markerStyle = m.selectedBackground(m.styles.selectedMarker)
	}
	space := " "
	if selected {
		space = m.selectedBackground(m.styles.value).Render(" ")
	}
	separator := m.styles.separator.Render(" │ ")
	if selected {
		separator = m.selectedBackground(m.styles.separator).Render(" │ ")
	}
	line := markerStyle.Render(marker) + space + strings.Join([]string{
		m.renderStatusCell(row.Status, status, selected),
		m.renderValueCell(row.Tool, tool, selected),
		m.renderValueCell(row.TargetPane(), target, selected),
		m.renderValueCell(row.SessionName, session, selected),
		m.renderValueCell(row.WindowName, window, selected),
		m.renderValueCell(row.Repo, repo, selected),
		m.renderTokensCell(row.TokensUsed, tokens, selected),
		m.renderContextCell(row.ContextLeftPct, ctx, selected),
		m.renderValueCell(prettyAge(row.WindowActivity), 0, selected),
	}, separator)
	if selected {
		return m.fitSelectedLine(line, width)
	}
	return fitLine(line, width)
}

func (m BoardModel) renderPreview(width, height int) []string {
	lines := make([]string, 0, height)
	if !m.rowsLoaded {
		return padLines([]string{m.styles.info.Render("loading preview...")}, height)
	}
	if m.selectedPaneID == "" {
		return padLines([]string{m.styles.info.Render("(no preview)")}, height)
	}
	if m.preview.PaneID == "" || m.preview.PaneID != m.selectedPaneID {
		return padLines([]string{m.styles.info.Render(fmt.Sprintf("loading preview for %s", m.selectedPaneID))}, height)
	}
	lines = appendWrappedLines(lines, wrapStyledLine(m.renderPreviewSummaryLine(), width), height)
	if meta := m.renderPreviewDetailLine(); meta != "" {
		lines = appendWrappedLines(lines, wrapStyledLine(meta, width), height)
	}
	if len(lines) >= height {
		return padLines(lines, height)
	}
	lines = append(lines, fitLine(m.styles.borderMuted.Render(strings.Repeat("─", width)), width))
	bodyLines := strings.Split(m.preview.Body, "\n")
	remaining := max(0, height-len(lines))
	for _, line := range bodyLines {
		if remaining == 0 {
			break
		}
		wrapped := wrapStyledLine(m.renderPreviewBodyLine(line), width)
		if len(wrapped) > remaining {
			wrapped = wrapped[:remaining]
		}
		lines = append(lines, wrapped...)
		remaining -= len(wrapped)
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

func boardPanelWidths(width int) (left, right int, horizontal bool) {
	const separatorWidth = 3

	if width < 96 {
		return width, width, false
	}

	contentWidth := width - separatorWidth
	left = contentWidth * 3 / 5
	right = contentWidth - left
	if left < 60 || right < 28 {
		return width, width, false
	}
	return left, right, true
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
	if width <= 0 {
		return text
	}
	text = ansi.Truncate(text, width, "")
	visible := lipgloss.Width(text)
	if visible < width {
		text += strings.Repeat(" ", width-visible)
	}
	return text
}

func wrapStyledLine(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	wrapped := ansi.Wrap(text, width, "")
	parts := strings.Split(wrapped, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, fitLine(part, width))
	}
	return lines
}

func appendWrappedLines(dst, src []string, limit int) []string {
	if limit <= 0 {
		return dst
	}
	remaining := limit - len(dst)
	if remaining <= 0 {
		return dst
	}
	if len(src) > remaining {
		src = src[:remaining]
	}
	return append(dst, src...)
}

func joinColumns(parts ...string) string {
	return strings.Join(parts, " │ ")
}

func (m BoardModel) renderStatusCell(st state.Status, width int, selected bool) string {
	style, ok := m.styles.statusByValue[st]
	if !ok {
		style = m.styles.value
	}
	if selected {
		style = m.selectedBackground(style)
	}
	return style.Render(fitCell(string(st), width))
}

func (m BoardModel) renderHeader(width int) []string {
	prompt := m.styles.helpKey.Render("Panefleet >") + " " + m.styles.value.Render(m.searchQuery)
	controls := m.renderControlsLine()
	switch {
	case m.err != nil:
		return []string{
			fitLine(prompt, width),
			fitLine(controls, width),
			fitLine(m.styles.error.Render("degraded: "+trimText(m.err.Error(), 64)), width),
		}
	case !m.rowsLoaded:
		return []string{
			fitLine(prompt, width),
			fitLine(controls, width),
			fitLine(m.styles.info.Render("loading panes..."), width),
		}
	}
	return []string{
		fitLine(prompt, width),
		fitLine(controls, width),
	}
}

func (m BoardModel) renderSectionBar(title, meta string) string {
	return m.styles.sectionTitle.Render(strings.ToUpper(title))
}

func (m BoardModel) renderPill(style lipgloss.Style, text string) string {
	return style.Render(strings.TrimSpace(text))
}

func (m BoardModel) renderStatusPill(st state.Status) string {
	style, ok := m.styles.statusPill[st]
	if !ok {
		style = m.styles.pill
	}
	return style.Render(string(st))
}

func (m BoardModel) renderInlineStatus(st state.Status) string {
	style, ok := m.styles.statusByValue[st]
	if !ok {
		style = m.styles.value
	}
	return style.Render(string(st))
}

func (m BoardModel) renderControlsLine() string {
	parts := []string{
		m.styles.helpKey.Render("[⏎]") + " " + m.styles.help.Render("jump"),
		m.styles.helpKey.Render("[ctrl+s]") + " " + m.styles.help.Render("stale"),
		m.styles.helpKey.Render("[esc]") + " " + m.styles.help.Render("quit"),
	}
	return strings.Join(parts, m.styles.separator.Render(" · "))
}

func (m *BoardModel) handleSearchKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	before := m.selectedPaneID
	switch {
	case isSearchClearLineKey(msg):
		if m.searchQuery == "" {
			return false, nil
		}
		m.searchQuery = ""
	case isSearchDeleteWordKey(msg):
		if m.searchQuery == "" {
			return false, nil
		}
		m.searchQuery = trimLastWord(m.searchQuery)
	case msg.Type == tea.KeyBackspace:
		if m.searchQuery == "" {
			return false, nil
		}
		m.searchQuery = trimLastRune(m.searchQuery)
	case msg.Type == tea.KeyRunes:
		m.searchQuery += string(msg.Runes)
	default:
		return false, nil
	}
	m.reconcileSelection()
	if m.selectedPaneID != "" && m.selectedPaneID != before {
		return true, m.requestPreviewCmd(m.selectedPaneID)
	}
	return true, nil
}

func (m BoardModel) filteredRows() []board.Row {
	query := strings.ToLower(strings.TrimSpace(m.searchQuery))
	if query == "" {
		return m.rows
	}
	filtered := make([]board.Row, 0, len(m.rows))
	for _, row := range m.rows {
		if rowMatchesQuery(row, query) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func rowMatchesQuery(row board.Row, query string) bool {
	fields := []string{
		row.PaneID,
		string(row.Status),
		row.Tool,
		row.TargetPane(),
		row.TargetWindow(),
		row.SessionName,
		row.WindowName,
		row.Repo,
		row.Path,
		row.Command,
		row.Title,
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

func trimLastRune(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}

func trimLastWord(s string) string {
	s = strings.TrimRightFunc(s, unicode.IsSpace)
	runes := []rune(s)
	for len(runes) > 0 && !unicode.IsSpace(runes[len(runes)-1]) {
		runes = runes[:len(runes)-1]
	}
	return strings.TrimRightFunc(string(runes), unicode.IsSpace)
}

func isSearchClearLineKey(msg tea.KeyMsg) bool {
	// Terminals do not agree on how ctrl+backspace is encoded. Depending on the
	// terminal/tmux stack Bubble Tea may surface it as ctrl+h, ctrl+u, ctrl+w,
	// or a dedicated ctrl+backspace string.
	switch msg.String() {
	case "ctrl+backspace", "ctrl+h", "ctrl+u", "ctrl+w":
		return true
	default:
		return false
	}
}

func isSearchDeleteWordKey(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyBackspace && msg.Alt
}

func (m *BoardModel) applyPaneState(st state.PaneState) {
	effective := st.Effective()
	for i := range m.rows {
		if m.rows[i].PaneID != st.PaneID {
			continue
		}
		m.rows[i].Status = effective.Status
		m.rows[i].StatusSource = effective.StatusSource
		m.rows[i].ReasonCode = effective.ReasonCode
		m.rows[i].ManualOverride = copyStatusPtr(st.ManualOverride)
		m.rows[i].LastTransitionAt = effective.LastTransitionAt
		return
	}
}

func copyStatusPtr(in *state.Status) *state.Status {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func (m BoardModel) renderTokensCell(tokensUsed *int, width int, selected bool) string {
	text := fitCell(optionalInt(tokensUsed), width)
	if tokensUsed == nil {
		style := m.styles.metricMuted
		if selected {
			style = m.selectedBackground(style)
		}
		return style.Render(text)
	}
	style := m.styles.metricValue
	if selected {
		style = m.selectedBackground(style)
	}
	return style.Render(text)
}

func (m BoardModel) renderContextCell(contextLeftPct *int, width int, selected bool) string {
	text := fitCell(optionalPercent(contextLeftPct), width)
	var style lipgloss.Style
	if contextLeftPct == nil {
		style = m.styles.metricMuted
	} else {
		switch {
		case *contextLeftPct >= 60:
			style = m.styles.metricValue
		case *contextLeftPct >= 30:
			style = m.styles.metricWarn
		default:
			style = m.styles.metricDanger
		}
	}
	if selected {
		style = m.selectedBackground(style)
	}
	return style.Render(text)
}

func (m BoardModel) renderValueCell(text string, width int, selected bool) string {
	style := m.styles.value
	if selected {
		style = m.selectedBackground(style).Foreground(m.styles.selectedRow.GetForeground()).Bold(true)
	}
	if width > 0 {
		text = fitCell(text, width)
	}
	return style.Render(text)
}

func (m BoardModel) selectedBackground(style lipgloss.Style) lipgloss.Style {
	return style.Copy().Background(m.styles.selectedRow.GetBackground())
}

func (m BoardModel) fitSelectedLine(text string, width int) string {
	text = ansi.Truncate(text, width, "")
	visible := lipgloss.Width(text)
	if visible < width {
		text += m.selectedBackground(m.styles.value).Render(strings.Repeat(" ", width-visible))
	}
	return text
}

func (m BoardModel) renderPreviewBodyLine(line string) string {
	lower := strings.ToLower(strings.TrimSpace(line))
	gutter := m.styles.previewGutter.Render("│ ")
	switch {
	case lower == "":
		return gutter
	case strings.HasPrefix(lower, "diff --git"), strings.HasPrefix(lower, "index "):
		return gutter + m.styles.previewHeading.Render(line)
	case strings.HasPrefix(lower, "@@ "):
		return gutter + m.styles.diffHunk.Render(line)
	case strings.HasPrefix(lower, "+++ "):
		return gutter + m.styles.diffAdd.Render(line)
	case strings.HasPrefix(lower, "--- "):
		return gutter + m.styles.diffRemove.Render(line)
	case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++ "):
		return gutter + m.styles.diffAdd.Render(line)
	case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "--- "):
		return gutter + m.styles.diffRemove.Render(line)
	case strings.HasPrefix(lower, "#"):
		return gutter + m.styles.previewHeading.Render(line)
	case strings.HasPrefix(lower, ">"):
		return gutter + m.styles.previewQuote.Render(line)
	case strings.HasPrefix(lower, "- "), strings.HasPrefix(lower, "* "), strings.HasPrefix(lower, "+ "):
		return gutter + m.styles.previewList.Render(line)
	case strings.HasPrefix(lower, "• "), strings.HasPrefix(lower, "› "), strings.HasPrefix(lower, "$ "):
		return gutter + m.styles.previewShell.Render(line)
	case strings.Contains(lower, "warning"):
		return gutter + m.styles.previewWarning.Render(line)
	case strings.Contains(lower, "error"):
		return gutter + m.styles.previewError.Render(line)
	default:
		return gutter + m.styles.previewCode.Render(line)
	}
}

func (m BoardModel) renderPreviewSummaryLine() string {
	parts := []string{
		m.renderInlineStatus(m.preview.Status),
		m.styles.accentValue.Render(m.preview.Tool),
		m.styles.value.Render(fmt.Sprintf("%s:%s.%s", m.preview.SessionName, m.preview.WindowIndex, m.preview.PaneIndex)),
	}
	if name := strings.TrimSpace(m.preview.WindowName); name != "" {
		parts = append(parts, m.styles.mutedValue.Render(name))
	}
	return strings.Join(parts, m.styles.separator.Render(" · "))
}

func (m BoardModel) renderPreviewDetailLine() string {
	parts := make([]string, 0, 3)
	if path := strings.TrimSpace(m.preview.Path); path != "" {
		parts = append(parts, m.styles.accentValue.Render(path))
	}
	if cmd := strings.TrimSpace(m.preview.Command); cmd != "" {
		parts = append(parts, m.styles.mutedValue.Render(cmd))
	}
	if title := strings.TrimSpace(m.preview.Title); title != "" {
		parts = append(parts, m.styles.mutedValue.Render(title))
	}
	return strings.Join(parts, m.styles.separator.Render(" · "))
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

func trimText(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	if limit == 1 {
		return string(runes[:1])
	}
	return string(runes[:limit-1]) + "…"
}
