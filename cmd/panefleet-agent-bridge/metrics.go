package main

import (
	"context"
	"fmt"
	"math"
	"strconv"
)

// codexTokenUsage extracts metrics from Codex app-server payloads.
// The mapper stays permissive on shape variations because these payloads have
// already drifted between nested and top-level forms.
func codexTokenUsage(payload map[string]any) (tokensUsed, contextWindow, contextLeftPct int, hasTokens, hasWindow bool) {
	params := mapValue(payload["params"])
	tokenUsage := mapValue(params["tokenUsage"])
	total := mapValue(tokenUsage["total"])

	tokensUsed, hasTokens = intValue(
		total["totalTokens"],
		tokenUsage["totalTokens"],
		params["totalTokens"],
		params["total_tokens"],
	)
	contextWindow, hasWindow = intValue(
		tokenUsage["modelContextWindow"],
		params["modelContextWindow"],
		params["model_context_window"],
	)
	if hasTokens && hasWindow && contextWindow > 0 {
		remaining := max(contextWindow-tokensUsed, 0)
		contextLeftPct = int(math.Round(float64(remaining) * 100 / float64(contextWindow)))
		contextLeftPct = min(contextLeftPct, 100)
	}

	return tokensUsed, contextWindow, contextLeftPct, hasTokens, hasWindow && contextWindow > 0
}

func applyCodexTokenUsage(ctx context.Context, pane, source, eventID string, payload map[string]any) error {
	tokensUsed, contextWindow, contextLeftPct, hasTokens, hasWindow := codexTokenUsage(payload)
	if !hasTokens {
		logDecision(source, pane, eventID, "ignored", "", "token usage payload missing total tokens", "")
		return nil
	}

	if err := setUsageMetrics(ctx, pane, tokensUsed, contextWindow, contextLeftPct, hasWindow); err != nil {
		logDecision(source, pane, eventID, "metrics_error", "", "token usage update failed", err.Error())
		return err
	}

	reason := "token usage updated"
	if hasWindow {
		reason = "token usage and context window updated"
	}
	logDecision(source, pane, eventID, "metrics_set", "", reason, "")
	return nil
}

func setUsageMetrics(ctx context.Context, pane string, tokensUsed, contextWindow, contextLeftPct int, hasWindow bool) error {
	args := []string{
		"metrics-set",
		"--pane", pane,
		"--tokens-used", strconv.Itoa(tokensUsed),
	}
	if hasWindow {
		args = append(
			args,
			"--context-window", strconv.Itoa(contextWindow),
			"--context-left-pct", strconv.Itoa(contextLeftPct),
		)
	}
	if err := runPanefleet(ctx, args...); err != nil {
		return fmt.Errorf("set usage metrics: %w", err)
	}
	return nil
}

func intValue(values ...any) (int, bool) {
	for _, value := range values {
		switch typed := value.(type) {
		case float64:
			if typed < 0 || math.Trunc(typed) != typed {
				continue
			}
			return int(typed), true
		case float32:
			if typed < 0 || math.Trunc(float64(typed)) != float64(typed) {
				continue
			}
			return int(typed), true
		case int:
			if typed < 0 {
				continue
			}
			return typed, true
		case int64:
			if typed < 0 {
				continue
			}
			return int(typed), true
		case int32:
			if typed < 0 {
				continue
			}
			return int(typed), true
		case string:
			if parsed, err := strconv.Atoi(typed); err == nil && parsed >= 0 {
				return parsed, true
			}
		}
	}

	return 0, false
}
