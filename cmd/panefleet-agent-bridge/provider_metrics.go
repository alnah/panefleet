package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type providerUsageMetrics struct {
	tokensUsed           int
	contextWindow        int
	contextLeftPercent   int
	hasTokens            bool
	hasContextWindowInfo bool
}

func applyClaudeTranscriptMetrics(ctx context.Context, pane, source, eventID string, payload map[string]any) error {
	metrics, ok, err := lookupClaudeTranscriptMetrics(payload)
	if err != nil {
		logDecision(source, pane, eventID, "metrics_error", "", "claude transcript metrics lookup failed", err.Error())
		return nil
	}
	if !ok {
		logDecision(source, pane, eventID, "ignored", "", "claude transcript metrics unavailable", "")
		return nil
	}
	if err := setUsageMetrics(ctx, pane, metrics.tokensUsed, metrics.contextWindow, metrics.contextLeftPercent, metrics.hasContextWindowInfo); err != nil {
		logDecision(source, pane, eventID, "metrics_error", "", "claude transcript metrics update failed", err.Error())
		return err
	}
	logDecision(source, pane, eventID, "metrics_set", "", "claude transcript metrics applied", "")
	return nil
}

func lookupClaudeTranscriptMetrics(payload map[string]any) (providerUsageMetrics, bool, error) {
	transcriptPath := strings.TrimSpace(stringValue(payload["transcript_path"]))
	if transcriptPath == "" {
		return providerUsageMetrics{}, false, nil
	}
	raw, err := os.ReadFile(transcriptPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return providerUsageMetrics{}, false, nil
		}
		return providerUsageMetrics{}, false, fmt.Errorf("read claude transcript %s: %w", transcriptPath, err)
	}

	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		message := mapValue(entry["message"])
		if stringValue(message["role"]) != "assistant" {
			continue
		}
		usage := mapValue(message["usage"])
		tokensUsed, hasTokens := claudeUsageTotal(usage)
		if !hasTokens {
			return providerUsageMetrics{}, false, nil
		}
		model := strings.TrimSpace(stringValue(message["model"]))
		metrics := providerUsageMetrics{
			tokensUsed: tokensUsed,
			hasTokens:  true,
		}
		if contextWindow, ok := providerContextWindow(model); ok {
			metrics.contextWindow = contextWindow
			metrics.contextLeftPercent = remainingContextPercent(tokensUsed, contextWindow)
			metrics.hasContextWindowInfo = true
		}
		return metrics, true, nil
	}

	return providerUsageMetrics{}, false, nil
}

func claudeUsageTotal(usage map[string]any) (int, bool) {
	inputTokens, hasInput := intValue(usage["input_tokens"])
	outputTokens, hasOutput := intValue(usage["output_tokens"])
	cacheCreate, _ := intValue(usage["cache_creation_input_tokens"])
	cacheRead, _ := intValue(usage["cache_read_input_tokens"])
	if !hasInput && !hasOutput {
		return 0, false
	}
	return inputTokens + outputTokens + cacheCreate + cacheRead, true
}

func applyOpenCodeUsageMetrics(ctx context.Context, pane, source, eventID string, payload map[string]any) error {
	metrics, ok := openCodeUsageMetrics(payload)
	if !ok {
		return nil
	}
	if err := setUsageMetrics(ctx, pane, metrics.tokensUsed, metrics.contextWindow, metrics.contextLeftPercent, metrics.hasContextWindowInfo); err != nil {
		logDecision(source, pane, eventID, "metrics_error", "", "opencode usage metrics update failed", err.Error())
		return err
	}
	logDecision(source, pane, eventID, "metrics_set", "", "opencode usage metrics applied", "")
	return nil
}

func openCodeUsageMetrics(payload map[string]any) (providerUsageMetrics, bool) {
	event := mapValue(payload["event"])
	properties := mapValue(event["properties"])

	type tokenCarrier struct {
		tokens any
		model  string
	}
	candidates := []tokenCarrier{
		{
			tokens: mapValue(properties["info"])["tokens"],
			model:  stringValue(mapValue(properties["info"])["modelID"]),
		},
		{
			tokens: mapValue(properties["part"])["tokens"],
			model:  stringValue(mapValue(properties["info"])["modelID"]),
		},
		{
			tokens: mapValue(mapValue(properties["info"])["model"])["tokens"],
			model:  stringValue(mapValue(mapValue(properties["info"])["model"])["modelID"]),
		},
		{
			tokens: mapValue(payload["tokens"])["total"],
			model:  stringValue(payload["modelID"]),
		},
	}

	for _, candidate := range candidates {
		tokenMap := mapValue(candidate.tokens)
		total, ok := intValue(tokenMap["total"], candidate.tokens)
		if !ok {
			continue
		}
		metrics := providerUsageMetrics{
			tokensUsed: total,
			hasTokens:  true,
		}
		if contextWindow, ok := providerContextWindow(candidate.model); ok {
			metrics.contextWindow = contextWindow
			metrics.contextLeftPercent = remainingContextPercent(total, contextWindow)
			metrics.hasContextWindowInfo = true
		}
		return metrics, true
	}

	return providerUsageMetrics{}, false
}

func providerContextWindow(model string) (int, bool) {
	model = strings.TrimSpace(strings.ToLower(model))
	if model == "" {
		return 0, false
	}

	if info, ok, err := lookupCodexModelInfo(model); err == nil && ok {
		effectiveWindow := info.ContextWindow
		if info.EffectiveContextWindowPercent > 0 && info.EffectiveContextWindowPercent < 100 {
			effectiveWindow = effectiveWindow * info.EffectiveContextWindowPercent / 100
		}
		if effectiveWindow > 0 {
			return effectiveWindow, true
		}
	}

	switch {
	case strings.HasPrefix(model, "claude-"):
		return 200000, true
	default:
		return 0, false
	}
}

func claudeTranscriptPath(payload map[string]any) string {
	return strings.TrimSpace(stringValue(payload["transcript_path"]))
}

func codexModelsCachePath() string {
	return filepath.Join(codexHomeDir(), "models_cache.json")
}
