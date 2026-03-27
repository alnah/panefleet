package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/alnah/panefleet/internal/state"
)

type boardPalette struct {
	name        string
	bg          string
	bgAlt       string
	fg          string
	muted       string
	border      string
	accent      string
	header      string
	selectionBG string
	selectionFG string
	run         string
	wait        string
	done        string
	err         string
	idle        string
	stale       string
	diffHeader  string
	diffAdd     string
	diffRemove  string
	diffHunk    string
}

type boardStyles struct {
	title          lipgloss.Style
	help           lipgloss.Style
	info           lipgloss.Style
	error          lipgloss.Style
	headerCell     lipgloss.Style
	separator      lipgloss.Style
	selectedRow    lipgloss.Style
	label          lipgloss.Style
	value          lipgloss.Style
	accentValue    lipgloss.Style
	mutedValue     lipgloss.Style
	borderStrong   lipgloss.Style
	previewCode    lipgloss.Style
	previewHeading lipgloss.Style
	previewList    lipgloss.Style
	previewQuote   lipgloss.Style
	previewWarning lipgloss.Style
	previewError   lipgloss.Style
	previewShell   lipgloss.Style
	statusByValue  map[state.Status]lipgloss.Style
}

func resolveBoardPalette(name string) boardPalette {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "panefleet-light":
		return boardPalette{"panefleet-light", "#f8fafc", "#eef2f7", "#1f2937", "#6b7280", "#d2d8e2", "#2563eb", "#1d4ed8", "#dbeafe", "#0f172a", "#15803d", "#b45309", "#64748b", "#dc2626", "#2563eb", "#94a3b8", "#2563eb", "#15803d", "#dc2626", "#1d4ed8"}
	case "dracula":
		return boardPalette{"dracula", "#282a36", "#343746", "#f8f8f2", "#6272a4", "#44475a", "#bd93f9", "#ff79c6", "#44475a", "#f8f8f2", "#50fa7b", "#f1fa8c", "#6272a4", "#ff5555", "#8be9fd", "#6272a4", "#ff79c6", "#50fa7b", "#ff5555", "#bd93f9"}
	case "catppuccin-mocha":
		return boardPalette{"catppuccin-mocha", "#1e1e2e", "#313244", "#cdd6f4", "#9399b2", "#45475a", "#89b4fa", "#f5c2e7", "#45475a", "#cdd6f4", "#a6e3a1", "#f9e2af", "#9399b2", "#f38ba8", "#89b4fa", "#6c7086", "#89b4fa", "#a6e3a1", "#f38ba8", "#f5c2e7"}
	case "tokyo-night":
		return boardPalette{"tokyo-night", "#1a1b26", "#24283b", "#c0caf5", "#565f89", "#414868", "#7aa2f7", "#bb9af7", "#283457", "#c0caf5", "#9ece6a", "#e0af68", "#565f89", "#f7768e", "#7dcfff", "#414868", "#7aa2f7", "#9ece6a", "#f7768e", "#bb9af7"}
	case "gruvbox-dark":
		return boardPalette{"gruvbox-dark", "#282828", "#3c3836", "#ebdbb2", "#928374", "#504945", "#83a598", "#fabd2f", "#3c3836", "#fbf1c7", "#b8bb26", "#fabd2f", "#928374", "#fb4934", "#83a598", "#7c6f64", "#83a598", "#b8bb26", "#fb4934", "#fabd2f"}
	case "nord":
		return boardPalette{"nord", "#2e3440", "#3b4252", "#e5e9f0", "#81a1c1", "#4c566a", "#88c0d0", "#81a1c1", "#434c5e", "#eceff4", "#a3be8c", "#ebcb8b", "#4c566a", "#bf616a", "#81a1c1", "#616e88", "#88c0d0", "#a3be8c", "#bf616a", "#81a1c1"}
	case "solarized-dark":
		return boardPalette{"solarized-dark", "#002b36", "#073642", "#93a1a1", "#657b83", "#586e75", "#268bd2", "#2aa198", "#073642", "#eee8d5", "#859900", "#b58900", "#657b83", "#dc322f", "#268bd2", "#586e75", "#268bd2", "#859900", "#dc322f", "#2aa198"}
	case "rose-pine":
		return boardPalette{"rose-pine", "#191724", "#26233a", "#e0def4", "#908caa", "#403d52", "#9ccfd8", "#c4a7e7", "#403d52", "#e0def4", "#9ccfd8", "#f6c177", "#908caa", "#eb6f92", "#c4a7e7", "#6e6a86", "#9ccfd8", "#31748f", "#eb6f92", "#c4a7e7"}
	case "monokai":
		return boardPalette{"monokai", "#272822", "#3e3d32", "#f8f8f2", "#75715e", "#49483e", "#66d9ef", "#a6e22e", "#49483e", "#f8f8f2", "#a6e22e", "#e6db74", "#75715e", "#f92672", "#66d9ef", "#75715e", "#66d9ef", "#a6e22e", "#f92672", "#e6db74"}
	case "github-dark":
		return boardPalette{"github-dark", "#0d1117", "#161b22", "#c9d1d9", "#8b949e", "#30363d", "#58a6ff", "#79c0ff", "#1f2a38", "#e6edf3", "#3fb950", "#d29922", "#8b949e", "#f85149", "#58a6ff", "#6e7681", "#79c0ff", "#3fb950", "#f85149", "#d29922"}
	default:
		return boardPalette{"panefleet-dark", "#11131a", "#181c25", "#e6e9ef", "#8b93a7", "#2b3241", "#7cc7ff", "#9fd6ff", "#263248", "#eff3ff", "#72e39b", "#ffd166", "#93a1b7", "#ff7a90", "#7fb0ff", "#768096", "#7cc7ff", "#72e39b", "#ff8da1", "#9fd6ff"}
	}
}

func newBoardStyles(themeName string) boardStyles {
	palette := resolveBoardPalette(themeName)
	statusByValue := map[state.Status]lipgloss.Style{
		state.StatusRun:   lipgloss.NewStyle().Foreground(lipgloss.Color(palette.run)).Bold(true),
		state.StatusWait:  lipgloss.NewStyle().Foreground(lipgloss.Color(palette.wait)).Bold(true),
		state.StatusDone:  lipgloss.NewStyle().Foreground(lipgloss.Color(palette.done)),
		state.StatusError: lipgloss.NewStyle().Foreground(lipgloss.Color(palette.err)).Bold(true),
		state.StatusIdle:  lipgloss.NewStyle().Foreground(lipgloss.Color(palette.idle)),
		state.StatusStale: lipgloss.NewStyle().Foreground(lipgloss.Color(palette.stale)).Faint(true),
	}
	return boardStyles{
		title:          lipgloss.NewStyle().Foreground(lipgloss.Color(palette.header)).Bold(true),
		help:           lipgloss.NewStyle().Foreground(lipgloss.Color(palette.muted)),
		info:           lipgloss.NewStyle().Foreground(lipgloss.Color(palette.muted)),
		error:          lipgloss.NewStyle().Foreground(lipgloss.Color(palette.err)).Bold(true),
		headerCell:     lipgloss.NewStyle().Foreground(lipgloss.Color(palette.header)).Bold(true),
		separator:      lipgloss.NewStyle().Foreground(lipgloss.Color(palette.border)),
		selectedRow:    lipgloss.NewStyle().Background(lipgloss.Color(palette.selectionBG)).Foreground(lipgloss.Color(palette.selectionFG)),
		label:          lipgloss.NewStyle().Foreground(lipgloss.Color(palette.muted)).Bold(true),
		value:          lipgloss.NewStyle().Foreground(lipgloss.Color(palette.fg)),
		accentValue:    lipgloss.NewStyle().Foreground(lipgloss.Color(palette.accent)),
		mutedValue:     lipgloss.NewStyle().Foreground(lipgloss.Color(palette.idle)),
		borderStrong:   lipgloss.NewStyle().Foreground(lipgloss.Color(palette.header)),
		previewCode:    lipgloss.NewStyle().Foreground(lipgloss.Color(palette.fg)),
		previewHeading: lipgloss.NewStyle().Foreground(lipgloss.Color(palette.header)).Bold(true),
		previewList:    lipgloss.NewStyle().Foreground(lipgloss.Color(palette.fg)),
		previewQuote:   lipgloss.NewStyle().Foreground(lipgloss.Color(palette.muted)),
		previewWarning: lipgloss.NewStyle().Foreground(lipgloss.Color(palette.wait)).Bold(true),
		previewError:   lipgloss.NewStyle().Foreground(lipgloss.Color(palette.err)).Bold(true),
		previewShell:   lipgloss.NewStyle().Foreground(lipgloss.Color(palette.accent)),
		statusByValue:  statusByValue,
	}
}
