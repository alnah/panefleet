package main

func codexTokenUsageMetrics(payload map[string]any) (tokensUsed int64, contextLeftPct int64, contextWindow int64, ok bool) {
	params := mapValue(payload["params"])
	tokenUsage := mapValue(params["tokenUsage"])
	total := mapValue(tokenUsage["total"])
	totalTokens, hasTotalTokens := int64Value(total["totalTokens"])
	if !hasTotalTokens || totalTokens < 0 {
		return 0, -1, 0, false
	}

	contextLeftPct = -1
	contextWindow, hasWindow := int64Value(tokenUsage["modelContextWindow"])
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
