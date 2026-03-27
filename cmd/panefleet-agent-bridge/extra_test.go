package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePaneOrSkipWhenUnresolved(t *testing.T) {
	t.Setenv("PANEFLEET_PANE", "")
	t.Setenv("TMUX_PANE", "")
	pane, rest, skip, err := parsePaneOrSkip("codex-notify", nil)
	if err != nil {
		t.Fatalf("parsePaneOrSkip: %v", err)
	}
	if pane != "" || len(rest) != 0 || !skip {
		t.Fatalf("unexpected parsePaneOrSkip result: pane=%q rest=%v skip=%v", pane, rest, skip)
	}
}

func TestMappingAndFlagsFallbacks(t *testing.T) {
	if got := mapOpenCodeToolExecuteAfter("failed", "{}"); got != statusError {
		t.Fatalf("mapOpenCodeToolExecuteAfter failed = %q, want ERROR", got)
	}
	if got := mapOpenCodePermissionReply(map[string]any{"type": "permission.replied", "decision": "unknown"}); got != "" {
		t.Fatalf("mapOpenCodePermissionReply unknown = %q, want empty", got)
	}
	if flags := activeFlags(map[string]any{"activeFlags": "invalid"}); flags != nil {
		t.Fatalf("activeFlags invalid shape = %v, want nil", flags)
	}
}

func TestEnsureEventLogDirAndAppendJSONLogRecordFailures(t *testing.T) {
	if ensureEventLogDir("") {
		t.Fatalf("ensureEventLogDir empty should be false")
	}

	base := t.TempDir()
	filePath := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if ensureEventLogDir(filepath.Join(filePath, "child")) {
		t.Fatalf("ensureEventLogDir should fail when parent is a file")
	}

	t.Setenv("PANEFLEET_EVENT_LOG_DIR", base)
	appendJSONLogRecord("bad", map[string]any{"bad": func() {}})
	if _, err := os.Stat(filepath.Join(base, "bad.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("expected no bad.jsonl when marshal fails, err=%v", err)
	}
}

func TestEventLogPathSanitizesSource(t *testing.T) {
	base := t.TempDir()
	got := eventLogPath(base, "../codex/app-server")
	want := filepath.Join(base, "codex_app-server.jsonl")
	if got != want {
		t.Fatalf("eventLogPath() = %q, want %q", got, want)
	}

	if path := eventLogPath(base, " \t "); path != "" {
		t.Fatalf("blank eventLogPath should be empty, got %q", path)
	}
}

func TestLogPayloadSkipsBlank(t *testing.T) {
	logDir := t.TempDir()
	t.Setenv("PANEFLEET_EVENT_LOG_DIR", logDir)
	logPayload("codex-notify", "%1", "evt", []byte(" \n\t "))
	if _, err := os.Stat(filepath.Join(logDir, "codex-notify.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("blank payload should not log, err=%v", err)
	}
}

func TestNextEventIDIsUnique(t *testing.T) {
	seen := make(map[string]struct{}, 1024)
	for range 1024 {
		id := nextEventID()
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate event id generated: %s", id)
		}
		if !strings.Contains(id, "-") {
			t.Fatalf("event id should contain sequence suffix, got %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestAppendJSONLogRecordWritesSanitizedPath(t *testing.T) {
	logDir := t.TempDir()
	t.Setenv("PANEFLEET_EVENT_LOG_DIR", logDir)

	appendJSONLogRecord("../claude/hook", map[string]string{"ok": "true"})

	sanitized := filepath.Join(logDir, "claude_hook.jsonl")
	if _, err := os.Stat(sanitized); err != nil {
		t.Fatalf("expected sanitized log file, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(logDir, "..", "claude", "hook.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("unexpected path traversal log file, err=%v", err)
	}
}
