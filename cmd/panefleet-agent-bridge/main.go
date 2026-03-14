package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	statusRun   = "RUN"
	statusWait  = "WAIT"
	statusDone  = "DONE"
	statusError = "ERROR"

	defaultBridgeTimeout = 2 * time.Second
)

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: %s <claude-hook|codex-app-server|opencode-event> [flags]", filepath.Base(os.Args[0]))
	}

	ctx := context.Background()
	var err error

	switch os.Args[1] {
	case "claude-hook":
		err = runClaudeHook(ctx, os.Args[2:])
	case "codex-app-server":
		err = runCodexAppServer(ctx, os.Args[2:])
	case "codex-notify":
		err = runCodexNotify(ctx, os.Args[2:])
	case "opencode-event":
		err = runOpenCodeEvent(ctx, os.Args[2:])
	default:
		fatalf("usage: %s <claude-hook|codex-app-server|codex-notify|opencode-event> [flags]", filepath.Base(os.Args[0]))
	}

	if err != nil {
		fatalf("%v", err)
	}
}

func runClaudeHook(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("claude-hook", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pane := fs.String("pane", defaultPane(), "tmux pane id")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *pane == "" {
		return nil
	}

	raw, err := readAll(os.Stdin)
	if err != nil {
		return nil
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	logPayload("claude-hook", *pane, raw)

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}

	event := stringValue(payload["hook_event_name"])
	if event == "" {
		event = stringValue(payload["event"])
	}
	lowerBlob := strings.ToLower(string(raw))

	switch {
	case containsString([]string{"PreToolUse", "PostToolUse", "UserPromptSubmit", "SessionStart"}, event):
		return setState(ctx, *pane, statusRun, "claude", "claude-hook")
	case containsString([]string{"Stop", "SubagentStop", "SessionEnd", "PreCompact"}, event):
		return setState(ctx, *pane, statusDone, "claude", "claude-hook")
	case event == "Notification":
		return setState(ctx, *pane, statusWait, "claude", "claude-hook")
	case containsAny(lowerBlob, "error", "failed"):
		return setState(ctx, *pane, statusError, "claude", "claude-hook")
	default:
		return nil
	}
}

func runCodexNotify(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("codex-notify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pane := fs.String("pane", defaultPane(), "tmux pane id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pane == "" {
		return nil
	}

	raw, err := notificationPayload(fs.Args())
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	logPayload("codex-notify", *pane, raw)

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}

	if stringValue(payload["type"]) == "agent-turn-complete" {
		return setState(ctx, *pane, statusDone, "codex", "codex-notify")
	}
	return nil
}

func runCodexAppServer(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("codex-app-server", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pane := fs.String("pane", defaultPane(), "tmux pane id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pane == "" {
		return nil
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			continue
		}
		logPayload("codex-app-server", *pane, []byte(line))
		if stringValue(payload["method"]) != "thread/status/changed" {
			continue
		}

		state := mapCodexStatus(payload)
		if state == "" {
			continue
		}
		if err := setState(ctx, *pane, state, "codex", "codex-app-server"); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read codex app-server stream: %w", err)
	}
	return nil
}

func runOpenCodeEvent(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("opencode-event", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pane := fs.String("pane", defaultPane(), "tmux pane id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pane == "" {
		return nil
	}

	raw, err := readAll(os.Stdin)
	if err != nil {
		return nil
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	logPayload("opencode-event", *pane, raw)

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}

	state := mapOpenCodeEvent(payload, strings.ToLower(string(raw)))
	if state == "" {
		return nil
	}

	return setState(ctx, *pane, state, "opencode", "opencode-plugin")
}

func notificationPayload(args []string) ([]byte, error) {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return []byte(args[0]), nil
	}
	return readAll(os.Stdin)
}

func mapCodexStatus(payload map[string]any) string {
	params := mapValue(payload["params"])
	status := mapValue(params["status"])
	statusType := stringValue(status["type"])

	switch statusType {
	case "active":
		if boolValue(status["waitingOnApproval"]) || containsString(activeFlags(status), "waitingOnApproval") {
			return statusWait
		}
		return statusRun
	case "idle":
		return statusDone
	case "systemError":
		return statusError
	default:
		return ""
	}
}

func mapOpenCodeEvent(payload map[string]any, lowerBlob string) string {
	event := mapValue(payload["event"])
	eventType := stringValue(payload["type"])
	if eventType == "" {
		eventType = stringValue(event["type"])
	}

	status := stringValue(payload["status"])
	if status == "" {
		status = stringValue(event["status"])
	}
	status = strings.ToLower(status)

	switch {
	case eventType == "session.idle":
		return statusDone
	case eventType == "session.error":
		return statusError
	case eventType == "session.status" && containsString([]string{"busy", "running", "active"}, status):
		return statusRun
	case strings.HasPrefix(eventType, "tool.execute.before"):
		return statusRun
	case strings.HasPrefix(eventType, "tool.execute.after"):
		if containsAny(status, "error", "failed") || containsAny(lowerBlob, "\"error\"", "\"failed\"") {
			return statusError
		}
		return statusRun
	case eventType == "permission.asked":
		return statusWait
	case eventType == "permission.replied":
		if permissionDecisionDenied(payload) {
			return statusError
		}
		if permissionDecisionApproved(payload) {
			return statusRun
		}
		return ""
	case containsAny(strings.ToLower(eventType), "error") || status == "error":
		return statusError
	default:
		return ""
	}
}

func permissionDecisionApproved(payload map[string]any) bool {
	return containsString([]string{"approve", "approved", "allow", "allowed", "accept", "accepted"}, permissionDecision(payload))
}

func permissionDecisionDenied(payload map[string]any) bool {
	return containsString([]string{"deny", "denied", "reject", "rejected", "block", "blocked"}, permissionDecision(payload))
}

func permissionDecision(payload map[string]any) string {
	event := mapValue(payload["event"])
	for _, candidate := range []string{
		stringValue(payload["decision"]),
		stringValue(event["decision"]),
		stringValue(payload["result"]),
		stringValue(event["result"]),
		stringValue(payload["response"]),
		stringValue(event["response"]),
	} {
		if candidate != "" {
			return strings.ToLower(candidate)
		}
	}
	return ""
}

func activeFlags(status map[string]any) []string {
	rawFlags, ok := status["activeFlags"]
	if !ok {
		return nil
	}

	values, ok := rawFlags.([]any)
	if !ok {
		return nil
	}

	flags := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok && text != "" {
			flags = append(flags, text)
		}
	}
	return flags
}

func setState(ctx context.Context, pane, status, tool, source string) error {
	args := []string{
		"state-set",
		"--pane", pane,
		"--status", status,
		"--tool", tool,
		"--source", source,
		"--updated-at", strconv.FormatInt(time.Now().Unix(), 10),
	}
	return runPanefleet(ctx, args...)
}

func clearState(ctx context.Context, pane string) error {
	return runPanefleet(ctx, "state-clear", "--pane", pane)
}

func runPanefleet(ctx context.Context, args ...string) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, bridgeTimeout())
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, panefleetBin(), args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("run panefleet %s: timeout after %s", strings.Join(args, " "), bridgeTimeout())
		}
		return fmt.Errorf("run panefleet %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func readAll(file *os.File) ([]byte, error) {
	data, err := ioReadAll(file)
	if err != nil && !errors.Is(err, context.Canceled) {
		return nil, err
	}
	return data, nil
}

func ioReadAll(file *os.File) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(file)
	return buf.Bytes(), err
}

func defaultPane() string {
	if pane := os.Getenv("PANEFLEET_PANE"); pane != "" {
		return pane
	}
	return os.Getenv("TMUX_PANE")
}

func panefleetBin() string {
	if bin := os.Getenv("PANEFLEET_BIN"); bin != "" {
		return bin
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "panefleet"
	}
	return filepath.Join(home, ".tmux", "plugins", "panefleet", "bin", "panefleet")
}

func bridgeTimeout() time.Duration {
	raw := os.Getenv("PANEFLEET_BRIDGE_TIMEOUT_MS")
	if raw == "" {
		return defaultBridgeTimeout
	}

	millis, err := strconv.Atoi(raw)
	if err != nil || millis <= 0 {
		return defaultBridgeTimeout
	}
	return time.Duration(millis) * time.Millisecond
}

func logPayload(source, pane string, raw []byte) {
	logDir := os.Getenv("PANEFLEET_EVENT_LOG_DIR")
	if logDir == "" {
		return
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return
	}
	if err := os.Chmod(logDir, 0o700); err != nil {
		return
	}

	record := struct {
		Timestamp string          `json:"ts"`
		Source    string          `json:"source"`
		Pane      string          `json:"pane,omitempty"`
		Payload   json.RawMessage `json:"payload"`
	}{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Source:    source,
		Pane:      pane,
		Payload:   json.RawMessage(bytes.TrimSpace(raw)),
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		return
	}

	path := filepath.Join(logDir, source+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return
	}
	_, _ = file.Write(append(encoded, '\n'))
}

func stringValue(v any) string {
	switch value := v.(type) {
	case string:
		return value
	default:
		return ""
	}
}

func boolValue(v any) bool {
	value, ok := v.(bool)
	return ok && value
}

func mapValue(v any) map[string]any {
	value, ok := v.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return value
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
