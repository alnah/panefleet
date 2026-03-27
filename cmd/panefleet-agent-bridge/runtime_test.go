package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func withStdinText(t *testing.T, text string, fn func()) {
	t.Helper()
	old := os.Stdin
	tmp := filepath.Join(t.TempDir(), "stdin.txt")
	if err := os.WriteFile(tmp, []byte(text), 0o600); err != nil {
		t.Fatalf("write stdin fixture: %v", err)
	}
	file, err := os.Open(tmp)
	if err != nil {
		t.Fatalf("open stdin fixture: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Fatalf("close stdin fixture: %v", err)
		}
	}()
	os.Stdin = file
	defer func() { os.Stdin = old }()
	fn()
}

func fakePanefleetBin(t *testing.T, script string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "panefleet.log")
	bin := filepath.Join(dir, "panefleet-fake")
	script = strings.ReplaceAll(script, "__LOG_PATH__", logPath)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake panefleet: %v", err)
	}
	return bin, logPath
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(raw)
}

func seedCodexState(t *testing.T, codexHome, threadID, model string, tokensUsed int, contextWindow, effectivePct int) {
	t.Helper()

	statePath := filepath.Join(codexHome, "state_7.sqlite")
	db, err := sql.Open("sqlite", statePath)
	if err != nil {
		t.Fatalf("open sqlite fixture: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
CREATE TABLE threads (
	id TEXT PRIMARY KEY,
	tokens_used INTEGER NOT NULL DEFAULT 0,
	model TEXT
);`); err != nil {
		t.Fatalf("create threads fixture: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO threads (id, tokens_used, model) VALUES (?, ?, ?)`, threadID, tokensUsed, model); err != nil {
		t.Fatalf("insert thread fixture: %v", err)
	}

	modelsCache := codexModelsCache{
		Models: []codexModelInfo{
			{
				Slug:                          model,
				ContextWindow:                 contextWindow,
				EffectiveContextWindowPercent: effectivePct,
			},
		},
	}
	raw, err := json.Marshal(modelsCache)
	if err != nil {
		t.Fatalf("marshal models cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "models_cache.json"), raw, 0o600); err != nil {
		t.Fatalf("write models cache: %v", err)
	}
}

func TestParsePaneArgsAndSkip(t *testing.T) {
	t.Setenv("PANEFLEET_PANE", "%9")
	pane, rest, err := parsePaneArgs("codex-notify", []string{"--pane", "%7", "{\"type\":\"x\"}"})
	if err != nil {
		t.Fatalf("parsePaneArgs: %v", err)
	}
	if pane != "%7" || len(rest) != 1 {
		t.Fatalf("unexpected parse result pane=%q rest=%v", pane, rest)
	}

	t.Setenv("PANEFLEET_PANE", "")
	t.Setenv("TMUX_PANE", "")
	_, _, skip, err := parsePaneOrSkip("codex-notify", nil)
	if err != nil {
		t.Fatalf("parsePaneOrSkip: %v", err)
	}
	if !skip {
		t.Fatalf("expected skip when pane unresolved")
	}
}

func TestNotificationPayload(t *testing.T) {
	withStdinText(t, `{"type":"agent-turn-complete"}`, func() {
		raw, err := notificationPayload(nil)
		if err != nil {
			t.Fatalf("notificationPayload stdin: %v", err)
		}
		if string(raw) != `{"type":"agent-turn-complete"}` {
			t.Fatalf("unexpected payload: %q", string(raw))
		}
	})

	raw, err := notificationPayload([]string{`{"type":"agent-turn-complete"}`})
	if err != nil {
		t.Fatalf("notificationPayload arg: %v", err)
	}
	if !strings.Contains(string(raw), "agent-turn-complete") {
		t.Fatalf("expected arg payload to be used")
	}
}

func TestRunCodexNotifyAndClaudeHookAndOpenCodeEvent(t *testing.T) {
	bin, logPath := fakePanefleetBin(t, `#!/bin/sh
echo "$@" >> "__LOG_PATH__"
exit 0
`)
	t.Setenv("PANEFLEET_BIN", bin)
	t.Setenv("PANEFLEET_EVENT_LOG_DIR", t.TempDir())
	t.Setenv("PANEFLEET_PANE", "%1")

	if err := runCodexNotify(context.Background(), []string{`{"type":"agent-turn-complete"}`}); err != nil {
		t.Fatalf("runCodexNotify: %v", err)
	}
	withStdinText(t, `{"hook_event_name":"Stop"}`, func() {
		if err := runClaudeHook(context.Background(), nil); err != nil {
			t.Fatalf("runClaudeHook: %v", err)
		}
	})
	withStdinText(t, `{"type":"permission.asked"}`, func() {
		if err := runOpenCodeEvent(context.Background(), nil); err != nil {
			t.Fatalf("runOpenCodeEvent: %v", err)
		}
	})

	log := readLog(t, logPath)
	if !strings.Contains(log, "ingest --pane %1 --source codex-notify --kind exit --exit-code 0") {
		t.Fatalf("codex notify ingest not logged: %s", log)
	}
	if !strings.Contains(log, "ingest --pane %1 --source claude-hook --kind exit --exit-code 0") {
		t.Fatalf("claude hook ingest not logged: %s", log)
	}
	if !strings.Contains(log, "ingest --pane %1 --source opencode-plugin --kind wait") {
		t.Fatalf("opencode ingest not logged: %s", log)
	}
}

func TestRunCodexNotifySetsThreadMetricsFromCodexState(t *testing.T) {
	bin, logPath := fakePanefleetBin(t, `#!/bin/sh
echo "$@" >> "__LOG_PATH__"
exit 0
`)
	codexHome := t.TempDir()
	seedCodexState(t, codexHome, "thread-1", "gpt-5.4", 12345, 272000, 95)

	t.Setenv("PANEFLEET_BIN", bin)
	t.Setenv("PANEFLEET_EVENT_LOG_DIR", t.TempDir())
	t.Setenv("PANEFLEET_PANE", "%6")
	t.Setenv("CODEX_HOME", codexHome)

	if err := runCodexNotify(context.Background(), []string{`{"type":"agent-turn-complete","thread-id":"thread-1"}`}); err != nil {
		t.Fatalf("runCodexNotify: %v", err)
	}

	log := readLog(t, logPath)
	if !strings.Contains(log, "ingest --pane %6 --source codex-notify --kind exit --exit-code 0") {
		t.Fatalf("codex notify ingest not logged: %s", log)
	}
	if !strings.Contains(log, "metrics-set --pane %6 --tokens-used 12345 --context-window 258400 --context-left-pct 95") {
		t.Fatalf("codex notify metrics not logged: %s", log)
	}
}

func TestRunCodexAppServerSetsStateAndMetrics(t *testing.T) {
	bin, logPath := fakePanefleetBin(t, `#!/bin/sh
echo "$@" >> "__LOG_PATH__"
exit 0
`)
	t.Setenv("PANEFLEET_BIN", bin)
	t.Setenv("PANEFLEET_EVENT_LOG_DIR", t.TempDir())
	t.Setenv("PANEFLEET_PANE", "%2")

	stream := strings.Join([]string{
		`{"method":"thread/status/changed","params":{"status":{"type":"active"}}}`,
		`{"method":"thread/tokenUsage/updated","params":{"tokenUsage":{"total":{"totalTokens":123},"modelContextWindow":1000}}}`,
		`{"method":"thread/tokenUsage/updated","params":{"tokenUsage":{"total":{"totalTokens":456}}}}`,
	}, "\n")
	withStdinText(t, stream, func() {
		if err := runCodexAppServer(context.Background(), nil); err != nil {
			t.Fatalf("runCodexAppServer: %v", err)
		}
	})

	log := readLog(t, logPath)
	if !strings.Contains(log, "ingest --pane %2 --source codex-app-server --kind start") {
		t.Fatalf("ingest start not present in log: %s", log)
	}
	if !strings.Contains(log, "metrics-set --pane %2 --tokens-used 123 --context-window 1000 --context-left-pct 88") {
		t.Fatalf("expected metrics-set with derived context percentage, got: %s", log)
	}
	if !strings.Contains(log, "metrics-set --pane %2 --tokens-used 456") {
		t.Fatalf("expected metrics-set to keep updating token totals without context window, got: %s", log)
	}
}

func TestRunPanefleetTimeoutAndReadAll(t *testing.T) {
	slowBin, _ := fakePanefleetBin(t, `#!/bin/sh
sleep 1
exit 0
`)
	t.Setenv("PANEFLEET_BIN", slowBin)
	t.Setenv("PANEFLEET_BRIDGE_TIMEOUT_MS", "10")
	err := runPanefleet(context.Background(), "ingest", "--pane", "%9", "--kind", "start")
	if err == nil || !strings.Contains(err.Error(), "timeout after") {
		t.Fatalf("expected timeout error, got: %v", err)
	}

	withStdinText(t, "abc", func() {
		data, err := readAll(os.Stdin)
		if err != nil {
			t.Fatalf("readAll: %v", err)
		}
		if string(data) != "abc" {
			t.Fatalf("unexpected readAll bytes: %q", string(data))
		}
	})
}

func TestUsageLineAndApplyMappedStateIgnored(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"/tmp/panefleet-agent-bridge"}
	defer func() { os.Args = oldArgs }()

	if got := usageLine(); !strings.Contains(got, "claude-hook|codex-app-server|codex-notify|opencode-event") {
		t.Fatalf("usageLine missing commands: %q", got)
	}
	if err := applyMappedState(context.Background(), "%1", "source", "event", "", "reason"); err != nil {
		t.Fatalf("applyMappedState with empty status should be nil, got: %v", err)
	}
}

func TestRunCodexNotifyAndHookUnmappedPaths(t *testing.T) {
	bin, logPath := fakePanefleetBin(t, `#!/bin/sh
echo "$@" >> "__LOG_PATH__"
exit 0
`)
	t.Setenv("PANEFLEET_BIN", bin)
	t.Setenv("PANEFLEET_EVENT_LOG_DIR", t.TempDir())
	t.Setenv("PANEFLEET_PANE", "%3")

	if err := runCodexNotify(context.Background(), []string{`{"type":"noop"}`}); err != nil {
		t.Fatalf("runCodexNotify noop: %v", err)
	}
	withStdinText(t, ``, func() {
		if err := runCodexNotify(context.Background(), nil); err != nil {
			t.Fatalf("runCodexNotify empty stdin: %v", err)
		}
	})
	withStdinText(t, `{"event":"Stop"}`, func() {
		if err := runClaudeHook(context.Background(), nil); err != nil {
			t.Fatalf("runClaudeHook fallback event key: %v", err)
		}
	})
	withStdinText(t, `{"hook_event_name":"Notification"}`, func() {
		if err := runClaudeHook(context.Background(), nil); err != nil {
			t.Fatalf("runClaudeHook unmapped: %v", err)
		}
	})
	withStdinText(t, `{`, func() {
		if err := runOpenCodeEvent(context.Background(), nil); err != nil {
			t.Fatalf("runOpenCodeEvent invalid json should not fail: %v", err)
		}
	})
	withStdinText(t, `{"type":"noop"}`, func() {
		if err := runOpenCodeEvent(context.Background(), nil); err != nil {
			t.Fatalf("runOpenCodeEvent unmapped: %v", err)
		}
	})

	log := readLog(t, logPath)
	if strings.Count(log, "ingest") != 1 || !strings.Contains(log, "--source claude-hook --kind exit --exit-code 0") {
		t.Fatalf("expected only claude Stop fallback to map to ingest exit, got %q", log)
	}
}

func TestRunCodexAppServerHandlesInvalidAndUnmappedLines(t *testing.T) {
	bin, logPath := fakePanefleetBin(t, `#!/bin/sh
echo "$@" >> "__LOG_PATH__"
exit 0
`)
	t.Setenv("PANEFLEET_BIN", bin)
	t.Setenv("PANEFLEET_EVENT_LOG_DIR", t.TempDir())
	t.Setenv("PANEFLEET_PANE", "%8")

	stream := strings.Join([]string{
		`{invalid`,
		`{"method":"thread/status/changed","params":{"status":{"type":"mystery"}}}`,
		`{"method":"thread/tokenUsage/updated","params":{"tokenUsage":{"total":{}}}}`,
		`{"method":"unknown/method"}`,
	}, "\n")
	withStdinText(t, stream, func() {
		if err := runCodexAppServer(context.Background(), nil); err != nil {
			t.Fatalf("runCodexAppServer: %v", err)
		}
	})

	if log := readLog(t, logPath); strings.TrimSpace(log) != "" {
		t.Fatalf("expected no panefleet command for ignored lines, got %q", log)
	}
}

func TestRunCodexAppServerFlagParseError(t *testing.T) {
	withStdinText(t, `{}`, func() {
		if err := runCodexAppServer(context.Background(), []string{"--unknown"}); err == nil {
			t.Fatalf("expected flag parse error")
		}
	})
}

func TestParsePaneArgsErrorAndReadLoggedPayload(t *testing.T) {
	if _, _, err := parsePaneArgs("codex-notify", []string{"--unknown"}); err == nil {
		t.Fatalf("expected parsePaneArgs error")
	}

	withStdinText(t, "{", func() {
		_, _, _, ok, err := readLoggedStdinJSONPayload("codex-notify", "%1")
		if err != nil {
			t.Fatalf("readLoggedStdinJSONPayload err: %v", err)
		}
		if ok {
			t.Fatalf("expected ok=false for invalid JSON payload")
		}
	})

	withStdinText(t, `{"hook_event_name":"Stop"}`, func() {
		if err := runClaudeHook(context.Background(), []string{"--unknown"}); err == nil {
			t.Fatalf("expected flag parse error for runClaudeHook")
		}
	})
}

func TestRunPanefleetNonTimeoutError(t *testing.T) {
	failBin, _ := fakePanefleetBin(t, `#!/bin/sh
exit 3
`)
	t.Setenv("PANEFLEET_BIN", failBin)
	t.Setenv("PANEFLEET_BRIDGE_TIMEOUT_MS", "5000")
	err := runPanefleet(context.Background(), "ingest", "--pane", "%7", "--kind", "start")
	if err == nil || strings.Contains(err.Error(), "timeout after") {
		t.Fatalf("expected non-timeout run error, got: %v", err)
	}
}

func TestApplyMappedStateErrorBranchAndReadAllError(t *testing.T) {
	failBin, _ := fakePanefleetBin(t, `#!/bin/sh
exit 5
`)
	t.Setenv("PANEFLEET_BIN", failBin)
	t.Setenv("PANEFLEET_BRIDGE_TIMEOUT_MS", "5000")
	if err := applyMappedState(context.Background(), "%1", "source", "event", statusRun, "reason"); err == nil {
		t.Fatalf("expected applyMappedState to surface setState failure")
	}

	file, err := os.CreateTemp(t.TempDir(), "closed-stdin-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	name := file.Name()
	_ = file.Close()
	closed, err := os.Open(name)
	if err != nil {
		t.Fatalf("open temp file: %v", err)
	}
	_ = closed.Close()
	if _, err := readAll(closed); err == nil {
		t.Fatalf("expected readAll error on closed file")
	}
}

func TestIngestStateInvokesPanefleetWithSupportedFlags(t *testing.T) {
	bin, logPath := fakePanefleetBin(t, `#!/bin/sh
echo "$@" >> "__LOG_PATH__"
exit 0
	`)
	t.Setenv("PANEFLEET_BIN", bin)
	t.Setenv("PANEFLEET_BRIDGE_TIMEOUT_MS", "5000")

	if err := ingestState(context.Background(), "%4", statusRun, "source"); err != nil {
		t.Fatalf("ingestState RUN: %v", err)
	}
	if err := ingestState(context.Background(), "%4", statusDone, "source"); err != nil {
		t.Fatalf("ingestState DONE: %v", err)
	}

	log := readLog(t, logPath)
	if !strings.Contains(log, "ingest --pane %4 --source source --kind start") {
		t.Fatalf("start ingest not logged: %s", log)
	}
	if !strings.Contains(log, "ingest --pane %4 --source source --kind exit --exit-code 0") {
		t.Fatalf("done ingest not logged: %s", log)
	}
	if strings.Contains(log, "state-set") || strings.Contains(log, "--tool") || strings.Contains(log, "--updated-at") {
		t.Fatalf("unsupported CLI flags/commands leaked into panefleet invocation: %s", log)
	}
}

func TestBridgeRejectsUnexpectedArgs(t *testing.T) {
	t.Setenv("PANEFLEET_PANE", "%1")

	if err := runClaudeHook(context.Background(), []string{"extra"}); err == nil {
		t.Fatalf("expected unexpected arg error for claude-hook")
	}
	if err := runCodexAppServer(context.Background(), []string{"extra"}); err == nil {
		t.Fatalf("expected unexpected arg error for codex-app-server")
	}
	if err := runOpenCodeEvent(context.Background(), []string{"extra"}); err == nil {
		t.Fatalf("expected unexpected arg error for opencode-event")
	}
	if _, err := notificationPayload([]string{`{"type":"agent-turn-complete"}`, "extra"}); err == nil {
		t.Fatalf("expected unexpected arg error for codex-notify payload")
	}
}

func TestRunPanefleetIncludesStderr(t *testing.T) {
	failBin, _ := fakePanefleetBin(t, `#!/bin/sh
echo "bridge failure" >&2
exit 4
`)
	t.Setenv("PANEFLEET_BIN", failBin)
	t.Setenv("PANEFLEET_BRIDGE_TIMEOUT_MS", "5000")

	err := runPanefleet(context.Background(), "ingest", "--pane", "%7", "--kind", "start")
	if err == nil || !strings.Contains(err.Error(), "bridge failure") {
		t.Fatalf("expected stderr in runPanefleet error, got: %v", err)
	}
}
