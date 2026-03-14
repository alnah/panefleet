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
	case "opencode-event":
		err = runOpenCodeEvent(ctx, os.Args[2:])
	default:
		fatalf("unknown subcommand: %s", os.Args[1])
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
	case containsAny(lowerBlob, "permission", "approval", "confirm"):
		return setState(ctx, *pane, statusWait, "claude", "claude-hook")
	case containsAny(lowerBlob, "error", "failed"):
		return setState(ctx, *pane, statusError, "claude", "claude-hook")
	case event == "Notification":
		return clearState(ctx, *pane)
	default:
		return nil
	}
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
	case eventType == "session.status" && containsString([]string{"busy", "running", "active"}, status):
		return statusRun
	case strings.HasPrefix(eventType, "tool.execute.before"):
		return statusRun
	case containsAny(strings.ToLower(eventType), "permission") || containsAny(lowerBlob, "permission", "approval"):
		return statusWait
	case containsAny(strings.ToLower(eventType), "error") || status == "error":
		return statusError
	default:
		return ""
	}
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
	cmd := exec.CommandContext(ctx, panefleetBin(), args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
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
