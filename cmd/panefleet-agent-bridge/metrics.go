package main

func codexTokenUsageMetrics(payload map[string]any) (tokensUsed int64, contextLeftPct int64, contextWindow int64, ok bool) {
	params := mapValue(payload["params"])
	tokenUsage := mapValue(params["tokenUsage"])
	total := mapValue(tokenUsage["total"])
	totalTokens, hasTotalTokens := firstInt64(
		total["totalTokens"],
		total["total_tokens"],
		tokenUsage["totalTokens"],
		tokenUsage["total_tokens"],
		params["totalTokens"],
		params["total_tokens"],
	)
	if !hasTotalTokens || totalTokens < 0 {
		return 0, -1, 0, false
	}

	contextLeftPct = -1
	contextWindow, hasWindow := firstInt64(
		tokenUsage["modelContextWindow"],
		tokenUsage["model_context_window"],
		params["modelContextWindow"],
		params["model_context_window"],
	)
	if hasWindow && contextWindow > 0 {
		remaining := contextWindow - totalTokens
		if remaining < 0 {
			remaining = 0
		}
		contextLeftPct = (remaining * 100) / contextWindow
		if contextLeftPct < 0 {
			contextLeftPct = 0
		}
		if contextLeftPct > 100 {
			contextLeftPct = 100
		}
	}

	return totalTokens, contextLeftPct, contextWindow, true
}

func firstInt64(values ...any) (int64, bool) {
	for _, value := range values {
		if parsed, ok := int64Value(value); ok {
			return parsed, true
		}
	}
	return 0, false
}
