package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type codexNotifyMetrics struct {
	tokensUsed           int
	contextWindow        int
	contextLeftPercent   int
	hasContextWindowInfo bool
}

type codexModelsCache struct {
	Models []codexModelInfo `json:"models"`
}

type codexModelInfo struct {
	Slug                          string `json:"slug"`
	ContextWindow                 int    `json:"context_window"`
	EffectiveContextWindowPercent int    `json:"effective_context_window_percent"`
}

func applyCodexNotifyMetrics(ctx context.Context, pane, source, eventID string, payload map[string]any) error {
	metrics, ok, err := lookupCodexNotifyMetrics(payload)
	if err != nil {
		logDecision(source, pane, eventID, "metrics_error", "", "codex notify metrics lookup failed", err.Error())
		return nil
	}
	if !ok {
		logDecision(source, pane, eventID, "ignored", "", "codex notify metrics unavailable", "")
		return nil
	}
	if err := setUsageMetrics(ctx, pane, metrics.tokensUsed, metrics.contextWindow, metrics.contextLeftPercent, metrics.hasContextWindowInfo); err != nil {
		logDecision(source, pane, eventID, "metrics_error", "", "codex notify metrics update failed", err.Error())
		return err
	}
	logDecision(source, pane, eventID, "metrics_set", "", "codex notify thread metrics applied", "")
	return nil
}

func lookupCodexNotifyMetrics(payload map[string]any) (codexNotifyMetrics, bool, error) {
	threadID := strings.TrimSpace(stringValue(payload["thread-id"]))
	if threadID == "" {
		threadID = strings.TrimSpace(stringValue(payload["thread_id"]))
	}
	if threadID == "" {
		return codexNotifyMetrics{}, false, nil
	}

	dbPath, err := latestCodexStateDBPath()
	if err != nil {
		return codexNotifyMetrics{}, false, err
	}

	threadState, err := readCodexThreadState(dbPath, threadID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return codexNotifyMetrics{}, false, nil
		}
		return codexNotifyMetrics{}, false, err
	}

	metrics := codexNotifyMetrics{tokensUsed: threadState.tokensUsed}
	modelInfo, ok, err := lookupCodexModelInfo(threadState.model)
	if err != nil {
		return codexNotifyMetrics{}, false, err
	}
	if ok {
		effectiveWindow := modelInfo.ContextWindow
		if modelInfo.EffectiveContextWindowPercent > 0 && modelInfo.EffectiveContextWindowPercent < 100 {
			effectiveWindow = effectiveWindow * modelInfo.EffectiveContextWindowPercent / 100
		}
		if effectiveWindow > 0 {
			metrics.contextWindow = effectiveWindow
			metrics.contextLeftPercent = remainingContextPercent(threadState.tokensUsed, effectiveWindow)
			metrics.hasContextWindowInfo = true
		}
	}

	return metrics, true, nil
}

type codexThreadState struct {
	tokensUsed int
	model      string
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
