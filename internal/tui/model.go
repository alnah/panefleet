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

type StateReader interface {
	StateList(ctx context.Context) ([]state.PaneState, error)
}

type statesMsg struct {
	states []state.PaneState
	err    error
}

type tickMsg time.Time

type Model struct {
	reader      StateReader
	interval    time.Duration
	states      []state.PaneState
	lastRefresh time.Time
	err         error
}

func New(reader StateReader, interval time.Duration) Model {
	if interval <= 0 {
		interval = 700 * time.Millisecond
	}
	return Model{
		reader:   reader,
		interval: interval,
		states:   make([]state.PaneState, 0),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchCmd(), tickCmd(m.interval))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			return m, m.fetchCmd()
		}
	case statesMsg:
		m.err = msg.err
		if msg.err == nil {
			m.states = msg.states
			m.lastRefresh = time.Now().UTC()
		}
	case tickMsg:
		return m, tea.Batch(m.fetchCmd(), tickCmd(m.interval))
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString("Panefleet TUI (v1)\n")
	b.WriteString("q: quit  r: refresh\n\n")
	if m.err != nil {
		b.WriteString(fmt.Sprintf("error: %v\n\n", m.err))
	}
	b.WriteString(fmt.Sprintf("panes: %d  last refresh: %s\n\n", len(m.states), formatRefresh(m.lastRefresh)))
	b.WriteString("pane   status   source             reason\n")
	b.WriteString("----   ------   -----------------  -----------------------\n")

	for _, st := range m.states {
		b.WriteString(fmt.Sprintf("%-6s %-7s %-18s %-24s\n", st.PaneID, st.Status, trim(st.StatusSource, 18), trim(st.ReasonCode, 24)))
	}
	if len(m.states) == 0 {
		b.WriteString("(no pane state yet)\n")
	}
	return b.String()
}

func (m Model) fetchCmd() tea.Cmd {
	return func() tea.Msg {
		states, err := m.reader.StateList(context.Background())
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
