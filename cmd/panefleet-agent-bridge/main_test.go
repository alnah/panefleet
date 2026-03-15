package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMapCodexStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{
			name: "active thread is run",
			payload: map[string]any{
				"params": map[string]any{
					"status": map[string]any{"type": "active"},
				},
			},
			want: statusRun,
		},
		{
			name: "waiting on approval flag is wait",
			payload: map[string]any{
				"params": map[string]any{
					"status": map[string]any{
						"type":        "active",
						"activeFlags": []any{"waitingOnApproval"},
					},
				},
			},
			want: statusWait,
		},
		{
			name: "legacy waitingOnApproval boolean is wait",
			payload: map[string]any{
				"params": map[string]any{
					"status": map[string]any{
						"type":              "active",
						"waitingOnApproval": true,
					},
				},
			},
			want: statusWait,
		},
		{
			name: "idle thread is done",
			payload: map[string]any{
				"params": map[string]any{
					"status": map[string]any{"type": "idle"},
				},
			},
			want: statusDone,
		},
		{
			name: "system error is error",
			payload: map[string]any{
				"params": map[string]any{
					"status": map[string]any{"type": "systemError"},
				},
			},
			want: statusError,
		},
		{
			name: "unknown status is ignored",
			payload: map[string]any{
				"params": map[string]any{
					"status": map[string]any{"type": "mystery"},
				},
			},
			want: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := mapCodexStatus(tc.payload)
			if got != tc.want {
				t.Fatalf("mapCodexStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMapClaudeHookEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		event     string
		lowerBlob string
		wantState string
	}{
		{
			name:      "user prompt submit is run",
			event:     "UserPromptSubmit",
			lowerBlob: "{}",
			wantState: statusRun,
		},
		{
			name:      "permission request is wait",
			event:     "PermissionRequest",
			lowerBlob: "{}",
			wantState: statusWait,
		},
		{
			name:      "notification is ignored",
			event:     "Notification",
			lowerBlob: "{}",
			wantState: "",
		},
		{
			name:      "post tool use is ignored",
			event:     "PostToolUse",
			lowerBlob: "{}",
			wantState: "",
		},
		{
			name:      "stop is done",
			event:     "Stop",
			lowerBlob: "{}",
			wantState: statusDone,
		},
		{
			name:      "subagent stop is ignored",
			event:     "SubagentStop",
			lowerBlob: "{}",
			wantState: "",
		},
		{
			name:      "error blob is error",
			event:     "Other",
			lowerBlob: `{"error":"boom"}`,
			wantState: statusError,
		},
		{
			name:      "unknown event is ignored",
			event:     "Other",
			lowerBlob: "{}",
			wantState: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, _ := mapClaudeHookEvent(tc.event, tc.lowerBlob)
			if got != tc.wantState {
				t.Fatalf("mapClaudeHookEvent() = %q, want %q", got, tc.wantState)
			}
		})
	}
}

func TestMapOpenCodeEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		payload    map[string]any
		lowerBlob string
		want      string
	}{
		{
			name:      "session idle is done",
			payload:   map[string]any{"type": "session.idle"},
			lowerBlob: "{}",
			want:      statusDone,
		},
		{
			name:      "session error is error",
			payload:   map[string]any{"type": "session.error"},
			lowerBlob: "{}",
			want:      statusError,
		},
		{
			name:      "busy session status is run",
			payload:   map[string]any{"type": "session.status", "status": "busy"},
			lowerBlob: "{}",
			want:      statusRun,
		},
		{
			name:      "tool execute before is run",
			payload:   map[string]any{"type": "tool.execute.before"},
			lowerBlob: "{}",
			want:      statusRun,
		},
		{
			name:      "tool execute after with error blob is error",
			payload:   map[string]any{"type": "tool.execute.after"},
			lowerBlob: `{"error":"boom"}`,
			want:      statusError,
		},
		{
			name:      "permission asked is wait",
			payload:   map[string]any{"type": "permission.asked"},
			lowerBlob: "{}",
			want:      statusWait,
		},
		{
			name:      "permission approved is run",
			payload:   map[string]any{"type": "permission.replied", "decision": "approved"},
			lowerBlob: "{}",
			want:      statusRun,
		},
		{
			name:      "permission denied is error",
			payload:   map[string]any{"type": "permission.replied", "decision": "denied"},
			lowerBlob: "{}",
			want:      statusError,
		},
		{
			name:      "unknown event is ignored",
			payload:   map[string]any{"type": "noop"},
			lowerBlob: "{}",
			want:      "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := mapOpenCodeEvent(tc.payload, tc.lowerBlob)
			if got != tc.want {
				t.Fatalf("mapOpenCodeEvent() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPermissionDecision(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"event": map[string]any{
			"result": "allowed",
		},
	}

	if got := permissionDecision(payload); got != "allowed" {
		t.Fatalf("permissionDecision() = %q, want %q", got, "allowed")
	}
	if !permissionDecisionApproved(payload) {
		t.Fatalf("permissionDecisionApproved() = false, want true")
	}
	if permissionDecisionDenied(payload) {
		t.Fatalf("permissionDecisionDenied() = true, want false")
	}
}

func TestActiveFlags(t *testing.T) {
	t.Parallel()

	got := activeFlags(map[string]any{
		"activeFlags": []any{"waitingOnApproval", "other"},
	})
	if len(got) != 2 || got[0] != "waitingOnApproval" || got[1] != "other" {
		t.Fatalf("activeFlags() = %#v, want waitingOnApproval/other", got)
	}
}

func TestBridgeTimeout(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("PANEFLEET_BRIDGE_TIMEOUT_MS", "")
		if got := bridgeTimeout(); got != defaultBridgeTimeout {
			t.Fatalf("bridgeTimeout() = %s, want %s", got, defaultBridgeTimeout)
		}
	})

	t.Run("explicit milliseconds", func(t *testing.T) {
		t.Setenv("PANEFLEET_BRIDGE_TIMEOUT_MS", "250")
		if got := bridgeTimeout(); got != 250*time.Millisecond {
			t.Fatalf("bridgeTimeout() = %s, want %s", got, 250*time.Millisecond)
		}
	})

	t.Run("invalid value falls back", func(t *testing.T) {
		t.Setenv("PANEFLEET_BRIDGE_TIMEOUT_MS", "nope")
		if got := bridgeTimeout(); got != defaultBridgeTimeout {
			t.Fatalf("bridgeTimeout() = %s, want %s", got, defaultBridgeTimeout)
		}
	})
}

func TestDefaultPane(t *testing.T) {
	t.Run("prefers panefleet pane", func(t *testing.T) {
		t.Setenv("PANEFLEET_PANE", "%11")
		t.Setenv("TMUX_PANE", "%99")
		if got := defaultPane(); got != "%11" {
			t.Fatalf("defaultPane() = %q, want %q", got, "%11")
		}
	})

	t.Run("falls back to tmux pane", func(t *testing.T) {
		t.Setenv("PANEFLEET_PANE", "")
		t.Setenv("TMUX_PANE", "%99")
		if got := defaultPane(); got != "%99" {
			t.Fatalf("defaultPane() = %q, want %q", got, "%99")
		}
	})
}

func TestPanefleetBin(t *testing.T) {
	t.Run("uses override", func(t *testing.T) {
		t.Setenv("PANEFLEET_BIN", "/tmp/panefleet")
		if got := panefleetBin(); got != "/tmp/panefleet" {
			t.Fatalf("panefleetBin() = %q, want %q", got, "/tmp/panefleet")
		}
	})

	t.Run("falls back to plugin path", func(t *testing.T) {
		oldHome := os.Getenv("HOME")
		t.Setenv("PANEFLEET_BIN", "")
		t.Setenv("HOME", "/tmp/home")
		got := panefleetBin()
		want := "/tmp/home/.tmux/plugins/panefleet/bin/panefleet"
		if got != want {
			t.Fatalf("panefleetBin() = %q, want %q (old HOME %q)", got, want, oldHome)
		}
	})
}

func TestDecodeLoggedJSONPayloadWritesCorrelatedLogs(t *testing.T) {
	logDir := t.TempDir()
	t.Setenv("PANEFLEET_EVENT_LOG_DIR", logDir)

	payload, eventID, ok := decodeLoggedJSONPayload("claude-hook", "%42", []byte(`{"hook_event_name":"Stop"}`))
	if !ok {
		t.Fatalf("decodeLoggedJSONPayload() = not ok, want ok")
	}
	if eventID == "" {
		t.Fatalf("decodeLoggedJSONPayload() returned empty eventID")
	}
	if stringValue(payload["hook_event_name"]) != "Stop" {
		t.Fatalf("decoded hook_event_name = %q, want Stop", stringValue(payload["hook_event_name"]))
	}

	logDecision("claude-hook", "%42", eventID, "state_set", statusDone, "hook completion event", "")

	data, err := os.ReadFile(filepath.Join(logDir, "claude-hook.jsonl"))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	text := string(data)
	if !containsAny(text, `"kind":"payload"`, `"event_id":"`+eventID+`"`) {
		t.Fatalf("payload log missing correlated event record: %s", text)
	}
	if !containsAny(text, `"kind":"decision"`, `"decision":"state_set"`, `"status":"DONE"`) {
		t.Fatalf("decision log missing state_set record: %s", text)
	}
}

func TestDecodeLoggedJSONPayloadLogsDecodeErrors(t *testing.T) {
	logDir := t.TempDir()
	t.Setenv("PANEFLEET_EVENT_LOG_DIR", logDir)

	_, eventID, ok := decodeLoggedJSONPayload("opencode-event", "%7", []byte(`{`))
	if ok {
		t.Fatalf("decodeLoggedJSONPayload() = ok, want false")
	}
	if eventID == "" {
		t.Fatalf("decodeLoggedJSONPayload() returned empty eventID")
	}

	data, err := os.ReadFile(filepath.Join(logDir, "opencode-event.jsonl"))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	text := string(data)
	if !containsAny(text, `"kind":"decision"`, `"decision":"decode_error"`, `"event_id":"`+eventID+`"`) {
		t.Fatalf("decode error log missing correlated record: %s", text)
	}
}
