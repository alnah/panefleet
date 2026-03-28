package board

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/alnah/panefleet/internal/tmuxctl"
)

type rowMetrics struct {
	TokensUsed     *int
	ContextLeftPct *int
}

type rowMetricsResolver interface {
	resolve(context.Context, tmuxctl.BoardPane) (rowMetrics, bool, error)
}

type codexMetricsResolver struct {
	listProcesses    func(context.Context) ([]processInfo, error)
	listOpenFiles    func(context.Context, int) ([]string, error)
	lookupThreadData func(string) (rowMetrics, bool, error)
}

type processInfo struct {
	PPID    int
	PID     int
	Command string
}

var rolloutThreadIDPattern = regexp.MustCompile(`rollout-[^-\n]+(?:-[^-]+)*-([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\.jsonl$`)

func newCodexMetricsResolver() *codexMetricsResolver {
	return &codexMetricsResolver{
		listProcesses:    listProcesses,
		listOpenFiles:    listOpenFiles,
		lookupThreadData: lookupCodexThreadMetrics,
	}
}

func (r *codexMetricsResolver) resolve(ctx context.Context, pane tmuxctl.BoardPane) (rowMetrics, bool, error) {
	if pane.PanePID <= 0 {
		return rowMetrics{}, false, nil
	}
	processes, err := r.listProcesses(ctx)
	if err != nil {
		return rowMetrics{}, false, err
	}
	codexPID, ok := findCodexDescendantPID(processes, pane.PanePID)
	if !ok {
		return rowMetrics{}, false, nil
	}
	openFiles, err := r.listOpenFiles(ctx, codexPID)
	if err != nil {
		return rowMetrics{}, false, err
	}
	threadID, ok := extractCodexThreadID(openFiles)
	if !ok {
		return rowMetrics{}, false, nil
	}
	return r.lookupThreadData(threadID)
}

func findCodexDescendantPID(processes []processInfo, rootPID int) (int, bool) {
	childrenByParent := make(map[int][]processInfo, len(processes))
	for _, proc := range processes {
		childrenByParent[proc.PPID] = append(childrenByParent[proc.PPID], proc)
	}

	queue := append([]processInfo(nil), childrenByParent[rootPID]...)
	for len(queue) > 0 {
		proc := queue[0]
		queue = queue[1:]
		if isCodexCommand(proc.Command) {
			return proc.PID, true
		}
		queue = append(queue, childrenByParent[proc.PID]...)
	}
	return 0, false
}

func isCodexCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	base := filepath.Base(fields[0])
	return strings.HasPrefix(base, "codex")
}

func extractCodexThreadID(openFiles []string) (string, bool) {
	for _, path := range openFiles {
		matches := rolloutThreadIDPattern.FindStringSubmatch(strings.TrimSpace(path))
		if len(matches) == 2 {
			return matches[1], true
		}
	}
	return "", false
}

func listProcesses(ctx context.Context) ([]processInfo, error) {
	out, err := exec.CommandContext(ctx, "ps", "-ax", "-o", "ppid=,pid=,command=").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list processes: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	processes := make([]processInfo, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		ppid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		processes = append(processes, processInfo{
			PPID:    ppid,
			PID:     pid,
			Command: strings.Join(fields[2:], " "),
		})
	}
	return processes, nil
}

func listOpenFiles(ctx context.Context, pid int) ([]string, error) {
	out, err := exec.CommandContext(ctx, "lsof", "-p", strconv.Itoa(pid)).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list open files for pid %d: %w (%s)", pid, err, strings.TrimSpace(string(out)))
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) <= 1 {
		return nil, nil
	}
	paths := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}
		paths = append(paths, strings.Join(fields[8:], " "))
	}
	return paths, nil
}

func lookupCodexThreadMetrics(threadID string) (rowMetrics, bool, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return rowMetrics{}, false, nil
	}

	dbPath, err := latestCodexStateDBPath()
	if err != nil {
		return rowMetrics{}, false, err
	}
	threadState, err := readCodexThreadState(dbPath, threadID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rowMetrics{}, false, nil
		}
		return rowMetrics{}, false, err
	}

	metrics := rowMetrics{TokensUsed: intPtr(threadState.tokensUsed)}
	modelInfo, ok, err := lookupCodexModelInfo(threadState.model)
	if err != nil {
		return rowMetrics{}, false, err
	}
	if !ok {
		return metrics, true, nil
	}

	effectiveWindow := modelInfo.ContextWindow
	if modelInfo.EffectiveContextWindowPercent > 0 && modelInfo.EffectiveContextWindowPercent < 100 {
		effectiveWindow = effectiveWindow * modelInfo.EffectiveContextWindowPercent / 100
	}
	if effectiveWindow > 0 {
		metrics.ContextLeftPct = intPtr(remainingContextPercent(threadState.tokensUsed, effectiveWindow))
	}
	return metrics, true, nil
}

type codexThreadState struct {
	tokensUsed int
	model      string
}

type codexModelsCache struct {
	Models []codexModelInfo `json:"models"`
}

type codexModelInfo struct {
	Slug                          string `json:"slug"`
	ContextWindow                 int    `json:"context_window"`
	EffectiveContextWindowPercent int    `json:"effective_context_window_percent"`
}

func readCodexThreadState(dbPath, threadID string) (codexThreadState, error) {
	db, err := sql.Open("sqlite", sqliteReadOnlyDSN(dbPath))
	if err != nil {
		return codexThreadState{}, fmt.Errorf("open codex state db %s: %w", dbPath, err)
	}
	defer db.Close()

	var state codexThreadState
	row := db.QueryRow(`SELECT tokens_used, COALESCE(model, '') FROM threads WHERE id = ?`, threadID)
	if err := row.Scan(&state.tokensUsed, &state.model); err != nil {
		return codexThreadState{}, fmt.Errorf("read codex thread %s: %w", threadID, err)
	}
	return state, nil
}

func lookupCodexModelInfo(model string) (codexModelInfo, bool, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return codexModelInfo{}, false, nil
	}

	cachePath := filepath.Join(codexHomeDir(), "models_cache.json")
	raw, err := os.ReadFile(cachePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return codexModelInfo{}, false, nil
		}
		return codexModelInfo{}, false, fmt.Errorf("read codex models cache %s: %w", cachePath, err)
	}

	var cache codexModelsCache
	if err := json.Unmarshal(raw, &cache); err != nil {
		return codexModelInfo{}, false, fmt.Errorf("decode codex models cache %s: %w", cachePath, err)
	}
	for _, info := range cache.Models {
		if strings.TrimSpace(info.Slug) == model {
			return info, true, nil
		}
	}
	return codexModelInfo{}, false, nil
}

func latestCodexStateDBPath() (string, error) {
	matches, err := filepath.Glob(filepath.Join(codexHomeDir(), "state_*.sqlite"))
	if err != nil {
		return "", fmt.Errorf("find codex state db: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("find codex state db: no state_*.sqlite under %s", codexHomeDir())
	}

	sort.Slice(matches, func(i, j int) bool {
		leftInfo, leftErr := os.Stat(matches[i])
		rightInfo, rightErr := os.Stat(matches[j])
		if leftErr != nil || rightErr != nil {
			return matches[i] > matches[j]
		}
		if leftInfo.ModTime().Equal(rightInfo.ModTime()) {
			return matches[i] > matches[j]
		}
		return leftInfo.ModTime().After(rightInfo.ModTime())
	})
	return matches[0], nil
}

func sqliteReadOnlyDSN(path string) string {
	return fmt.Sprintf("file:%s?mode=ro", path)
}

func codexHomeDir() string {
	if home := strings.TrimSpace(os.Getenv("CODEX_HOME")); home != "" {
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".codex"
	}
	return filepath.Join(home, ".codex")
}

func remainingContextPercent(tokensUsed, effectiveWindow int) int {
	if effectiveWindow <= 0 {
		return 0
	}
	if tokensUsed <= 0 {
		return 100
	}
	remaining := effectiveWindow - tokensUsed
	if remaining <= 0 {
		return 0
	}
	return (remaining*100 + effectiveWindow/2) / effectiveWindow
}

func intPtr(v int) *int {
	return &v
}
