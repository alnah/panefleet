package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alnah/panefleet/internal/state"
)

// StateReader defines the runtime operations needed by the fallback TUI.
type StateReader interface {
	StateList(ctx context.Context) ([]state.PaneState, error)
	SetOverride(ctx context.Context, paneID string, target state.Status, source string) (state.PaneState, error)
	ClearOverride(ctx context.Context, paneID, source string) (state.PaneState, error)
	KillPane(ctx context.Context, paneID string) error
	RespawnPane(ctx context.Context, paneID string) error
}

type statesMsg struct {
	states []state.PaneState
	err    error
}

type tickMsg time.Time
type stateUpdatedMsg struct{}
type actionMsg struct {
	err error
}

// Model stores UI state for the fallback Bubble Tea application.
type Model struct {
	reader        StateReader
	interval      time.Duration
	opTimeout     time.Duration
	updates       <-chan state.PaneState
	states        []state.PaneState
	selected      int
	lastRefresh   time.Time
	fetching      bool
	acting        bool
	refreshQueued bool
	err           error
}

// New constructs the lightweight fallback TUI model used by the Go runtime
// path for direct state operations and diagnostics.
func New(reader StateReader, interval time.Duration, updates <-chan state.PaneState) Model {
	if interval <= 0 {
		interval = 700 * time.Millisecond
	}
	return Model{
		reader:    reader,
		interval:  interval,
		opTimeout: 5 * time.Second,
		updates:   updates,
		states:    make([]state.PaneState, 0),
		fetching:  true,
	}
}

// Init schedules first fetch + periodic updates so the view starts with live
// state even when no key interaction happened yet.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.fetchCmd(), tickCmd(m.interval)}
	if m.updates != nil {
		cmds = append(cmds, waitForUpdateCmd(m.updates))
	}
	return tea.Batch(cmds...)
}

// Update applies key interactions and refresh messages while keeping action
// commands side-effect isolated in tea.Cmd closures.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.states)-1 {
				m.selected++
			}
		case "r":
			if !m.fetching {
				m.fetching = true
				return m, m.fetchCmd()
			}
			m.refreshQueued = true
		case "s":
			if st, ok := m.selectedState(); ok && !m.acting {
				m.acting = true
				return m, m.toggleStaleOverrideCmd(st)
			}
		case "x":
			if st, ok := m.selectedState(); ok && !m.acting {
				m.acting = true
				return m, m.respawnCmd(st.PaneID)
			}
		case "d":
			if st, ok := m.selectedState(); ok && !m.acting {
				m.acting = true
				return m, m.killCmd(st.PaneID)
			}
		}
	case statesMsg:
		m.fetching = false
		m.err = msg.err
		if msg.err == nil {
			m.states = msg.states
			if m.selected >= len(m.states) && len(m.states) > 0 {
				m.selected = len(m.states) - 1
			}
			m.lastRefresh = time.Now().UTC()
		}
		if m.refreshQueued {
			m.refreshQueued = false
			m.fetching = true
			return m, m.fetchCmd()
		}
	case tickMsg:
		if m.fetching {
			return m, tickCmd(m.interval)
		}
		m.fetching = true
		return m, tea.Batch(m.fetchCmd(), tickCmd(m.interval))
	case stateUpdatedMsg:
		if m.fetching {
			m.refreshQueued = true
			if m.updates != nil {
				return m, waitForUpdateCmd(m.updates)
			}
			return m, nil
		}
		m.fetching = true
		if m.updates != nil {
			return m, tea.Batch(m.fetchCmd(), waitForUpdateCmd(m.updates))
		}
		return m, m.fetchCmd()
	case actionMsg:
		m.acting = false
		m.err = msg.err
		if m.fetching {
			m.refreshQueued = true
			return m, nil
		}
		m.fetching = true
		return m, m.fetchCmd()
	}
	return m, nil
}

// View renders a stable tabular fallback UI that remains readable in plain
// terminal themes and CI logs.
func (m Model) View() string {
	var b strings.Builder
	b.WriteString("Panefleet TUI (v1)\n")
	b.WriteString("q: quit  r: refresh  j/k: move  s: stale toggle  x: respawn  d: kill\n\n")
	if m.err != nil {
		b.WriteString(fmt.Sprintf("error: %v\n\n", m.err))
	}
	b.WriteString(fmt.Sprintf("panes: %d  last refresh: %s\n\n", len(m.states), formatRefresh(m.lastRefresh)))
	b.WriteString("sel pane   status   source             reason\n")
	b.WriteString("--- ----   ------   -----------------  -----------------------\n")

	for i, st := range m.states {
		marker := " "
		if i == m.selected {
			marker = ">"
		}
		b.WriteString(fmt.Sprintf("%-3s %-6s %-7s %-18s %-24s\n", marker, st.PaneID, st.Status, trim(st.StatusSource, 18), trim(st.ReasonCode, 24)))
	}
	if len(m.states) == 0 {
		b.WriteString("(no pane state yet)\n")
	}
	return b.String()
}

func (m Model) fetchCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), m.opTimeout)
		defer cancel()

		states, err := m.reader.StateList(ctx)
		if err != nil {
			return statesMsg{err: err}
		}
		sort.Slice(states, func(i, j int) bool {
			pi := priority(states[i].Status)
			pj := priority(states[j].Status)
			if pi != pj {
				return pi < pj
			}
			return states[i].PaneID < states[j].PaneID
		})
		return statesMsg{states: states}
	}
}

func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func waitForUpdateCmd(ch <-chan state.PaneState) tea.Cmd {
	return func() tea.Msg {
		_, ok := <-ch
		if !ok {
			return nil
		}
		return stateUpdatedMsg{}
	}
}

func (m Model) selectedState() (state.PaneState, bool) {
	if len(m.states) == 0 || m.selected < 0 || m.selected >= len(m.states) {
		return state.PaneState{}, false
	}
	return m.states[m.selected], true
}

func (m Model) toggleStaleOverrideCmd(st state.PaneState) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), m.opTimeout)
		defer cancel()
		var err error
		if st.ManualOverride != nil && *st.ManualOverride == state.StatusStale {
			_, err = m.reader.ClearOverride(ctx, st.PaneID, "tui")
		} else {
			_, err = m.reader.SetOverride(ctx, st.PaneID, state.StatusStale, "tui")
		}
		return actionMsg{err: err}
	}
}

func (m Model) respawnCmd(paneID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), m.opTimeout)
		defer cancel()
		err := m.reader.RespawnPane(ctx, paneID)
		return actionMsg{err: err}
	}
}

func (m Model) killCmd(paneID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), m.opTimeout)
		defer cancel()
		err := m.reader.KillPane(ctx, paneID)
		return actionMsg{err: err}
	}
}

func priority(s state.Status) int {
	switch s {
	case state.StatusWait:
		return 1
	case state.StatusRun:
		return 2
	case state.StatusError:
		return 3
	case state.StatusDone:
		return 4
	case state.StatusIdle:
		return 5
	case state.StatusStale:
		return 6
	default:
		return 7
	}
}

func formatRefresh(ts time.Time) string {
	if ts.IsZero() {
		return "-"
	}
	return ts.Format(time.RFC3339)
}

func trim(v string, max int) string {
	if max <= 3 || len(v) <= max {
		return v
	}
	return v[:max-3] + "..."
}
