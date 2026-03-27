package main

import (
	"context"
	"strings"
	"testing"
)

func TestCodexTokenUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		payload            map[string]any
		wantTokens         int
		wantContextWindow  int
		wantContextLeftPct int
		wantHasTokens      bool
		wantHasWindow      bool
	}{
		{
			name: "nested token usage computes rounded context left",
			payload: map[string]any{
				"params": map[string]any{
					"tokenUsage": map[string]any{
						"total": map[string]any{
							"totalTokens": float64(123),
						},
						"modelContextWindow": float64(1000),
					},
				},
			},
			wantTokens:         123,
			wantContextWindow:  1000,
			wantContextLeftPct: 88,
			wantHasTokens:      true,
			wantHasWindow:      true,
		},
		{
			name: "top level fallbacks are accepted",
			payload: map[string]any{
				"params": map[string]any{
					"totalTokens":        "500",
					"modelContextWindow": "2000",
				},
			},
			wantTokens:         500,
			wantContextWindow:  2000,
			wantContextLeftPct: 75,
			wantHasTokens:      true,
			wantHasWindow:      true,
		},
		{
			name: "missing context window still returns tokens",
			payload: map[string]any{
				"params": map[string]any{
					"tokenUsage": map[string]any{
						"total": map[string]any{
							"totalTokens": float64(456),
						},
					},
				},
			},
			wantTokens:         456,
			wantContextWindow:  0,
			wantContextLeftPct: 0,
			wantHasTokens:      true,
			wantHasWindow:      false,
		},
		{
			name: "missing totals are ignored",
			payload: map[string]any{
				"params": map[string]any{
					"tokenUsage": map[string]any{},
				},
			},
			wantHasTokens: false,
			wantHasWindow: false,
		},
		{
			name: "context percentage floors at zero when over budget",
			payload: map[string]any{
				"params": map[string]any{
					"tokenUsage": map[string]any{
						"total": map[string]any{
							"totalTokens": float64(2200),
						},
						"modelContextWindow": float64(2000),
					},
				},
			},
			wantTokens:         2200,
			wantContextWindow:  2000,
			wantContextLeftPct: 0,
			wantHasTokens:      true,
			wantHasWindow:      true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotTokens, gotContextWindow, gotContextLeftPct, gotHasTokens, gotHasWindow := codexTokenUsage(tc.payload)
			if gotTokens != tc.wantTokens || gotContextWindow != tc.wantContextWindow || gotContextLeftPct != tc.wantContextLeftPct || gotHasTokens != tc.wantHasTokens || gotHasWindow != tc.wantHasWindow {
				t.Fatalf("codexTokenUsage() = (%d, %d, %d, %t, %t), want (%d, %d, %d, %t, %t)", gotTokens, gotContextWindow, gotContextLeftPct, gotHasTokens, gotHasWindow, tc.wantTokens, tc.wantContextWindow, tc.wantContextLeftPct, tc.wantHasTokens, tc.wantHasWindow)
			}
		})
	}
}

func TestApplyCodexTokenUsage(t *testing.T) {
	bin, logPath := fakePanefleetBin(t, `#!/bin/sh
echo "$@" >> "__LOG_PATH__"
exit 0
`)
	t.Setenv("PANEFLEET_BIN", bin)
	t.Setenv("PANEFLEET_BRIDGE_TIMEOUT_MS", "5000")

	payload := map[string]any{
		"params": map[string]any{
			"tokenUsage": map[string]any{
				"total": map[string]any{
					"totalTokens": float64(321),
				},
				"modelContextWindow": float64(1000),
			},
		},
	}
	if err := applyCodexTokenUsage(context.Background(), "%7", "codex-app-server", "evt-1", payload); err != nil {
		t.Fatalf("applyCodexTokenUsage: %v", err)
	}

	if log := readLog(t, logPath); !strings.Contains(log, "metrics-set --pane %7 --tokens-used 321 --context-window 1000 --context-left-pct 68") {
		t.Fatalf("expected metrics-set invocation, got %q", log)
	}
}
