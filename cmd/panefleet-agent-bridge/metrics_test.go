package main

import "testing"

func TestCodexTokenUsageMetrics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		payload        map[string]any
		wantTokens     int64
		wantCtxLeftPct int64
		wantContextWin int64
		wantOk         bool
	}{
		{
			name: "token usage with context window",
			payload: map[string]any{
				"params": map[string]any{
					"tokenUsage": map[string]any{
						"total": map[string]any{
							"totalTokens": float64(12000),
						},
						"modelContextWindow": float64(128000),
					},
				},
			},
			wantTokens:     12000,
			wantCtxLeftPct: 90,
			wantContextWin: 128000,
			wantOk:         true,
		},
		{
			name: "token usage without context window",
			payload: map[string]any{
				"params": map[string]any{
					"tokenUsage": map[string]any{
						"total": map[string]any{
							"totalTokens": float64(9000),
						},
					},
				},
			},
			wantTokens:     9000,
			wantCtxLeftPct: -1,
			wantContextWin: 0,
			wantOk:         true,
		},
		{
			name: "supports flat token usage payload fields",
			payload: map[string]any{
				"params": map[string]any{
					"tokenUsage": map[string]any{
						"totalTokens":        float64(64000),
						"modelContextWindow": float64(128000),
					},
				},
			},
			wantTokens:     64000,
			wantCtxLeftPct: 50,
			wantContextWin: 128000,
			wantOk:         true,
		},
		{
			name: "supports string number token fields",
			payload: map[string]any{
				"params": map[string]any{
					"tokenUsage": map[string]any{
						"total": map[string]any{
							"totalTokens": "32000",
						},
						"model_context_window": "128000",
					},
				},
			},
			wantTokens:     32000,
			wantCtxLeftPct: 75,
			wantContextWin: 128000,
			wantOk:         true,
		},
		{
			name: "missing total tokens is ignored",
			payload: map[string]any{
				"params": map[string]any{
					"tokenUsage": map[string]any{
						"total": map[string]any{},
					},
				},
			},
			wantOk: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotTokens, gotCtxLeftPct, gotContextWin, gotOk := codexTokenUsageMetrics(tc.payload)
			if gotOk != tc.wantOk {
				t.Fatalf("codexTokenUsageMetrics() ok = %v, want %v", gotOk, tc.wantOk)
			}
			if !tc.wantOk {
				return
			}
			if gotTokens != tc.wantTokens {
				t.Fatalf("codexTokenUsageMetrics() tokens = %d, want %d", gotTokens, tc.wantTokens)
			}
			if gotCtxLeftPct != tc.wantCtxLeftPct {
				t.Fatalf("codexTokenUsageMetrics() context-left-pct = %d, want %d", gotCtxLeftPct, tc.wantCtxLeftPct)
			}
			if gotContextWin != tc.wantContextWin {
				t.Fatalf("codexTokenUsageMetrics() context-window = %d, want %d", gotContextWin, tc.wantContextWin)
			}
		})
	}
}
