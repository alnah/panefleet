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

// panefleet-agent-bridge converts provider-specific events into panefleet states.
// It exists to keep tmux-side heuristics simple while allowing explicit state
// updates when providers expose machine-readable events.

const (
	statusRun   = "RUN"
	statusWait  = "WAIT"
	statusDone  = "DONE"
	statusError = "ERROR"

	defaultBridgeTimeout = 2 * time.Second
	// defaultScannerBufferSize keeps Scanner's small allocation behavior for common
	// payloads while maxScannerTokenBytes prevents long JSON events from failing.
	defaultScannerBufferSize = 64 * 1024
	maxScannerTokenBytes     = 2 * 1024 * 1024
)

func usageLine() string {
	return fmt.Sprintf("usage: %s <claude-hook|codex-app-server|codex-notify|opencode-event> [flags]", filepath.Base(os.Args[0]))
}

func main() {
	if len(os.Args) < 2 {
		fatalf("%s", usageLine())
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
		fatalf("%s", usageLine())
	}

	if err != nil {
		fatalf("%v", err)
	}
}

// runClaudeHook maps Claude hook payloads to panefleet states.
// It is intentionally tolerant of missing/partial payloads so Claude flows are
// not blocked when hooks misfire or emit unknown events.
func runClaudeHook(ctx context.Context, args []string) error {
	pane, _, skip, err := parsePaneOrSkip("claude-hook", args)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	payload, raw, eventID, ok, err := readLoggedStdinJSONPayload("claude-hook", pane)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	event := stringValue(payload["hook_event_name"])
	if event == "" {
		event = stringValue(payload["event"])
	}

	state, reason := mapClaudeHookEvent(event, strings.ToLower(string(raw)))
	if state == "" {
		logDecision("claude-hook", pane, eventID, "ignored", "", "unmapped hook event", "")
		return nil
	}
	return applyMappedState(ctx, pane, "claude", "claude-hook", eventID, state, reason)
}

// runCodexNotify handles one-shot Codex notifications that represent completion.
// This path exists because notify events may arrive without a long-lived stream.
func runCodexNotify(ctx context.Context, args []string) error {
	pane, rest, skip, err := parsePaneOrSkip("codex-notify", args)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	raw, err := notificationPayload(rest)
	if err != nil {
		return err
	}
	payload, eventID, ok := decodeLoggedJSONPayload("codex-notify", pane, raw)
	if !ok {
		return nil
	}

	if stringValue(payload["type"]) == "agent-turn-complete" {
		return applyMappedState(ctx, pane, "codex", "codex-notify", eventID, statusDone, "notify agent-turn-complete")
	}
	logDecision("codex-notify", pane, eventID, "ignored", "", "notify payload type not mapped", "")
	return nil
}

// runCodexAppServer consumes Codex status-change stream events and updates state.
// Non-status events are logged and ignored to keep the state machine stable.
func runCodexAppServer(ctx context.Context, args []string) error {
	pane, _, skip, err := parsePaneOrSkip("codex-app-server", args)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, defaultScannerBufferSize), maxScannerTokenBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		payload, eventID, ok := decodeLoggedJSONPayload("codex-app-server", pane, []byte(line))
		if !ok {
			continue
		}
		if stringValue(payload["method"]) != "thread/status/changed" {
			logDecision("codex-app-server", pane, eventID, "ignored", "", "non-status app-server method", "")
			continue
		}

		state := mapCodexStatus(payload)
		if state == "" {
			logDecision("codex-app-server", pane, eventID, "ignored", "", "status payload unmapped", "")
			continue
		}
		if err := applyMappedState(ctx, pane, "codex", "codex-app-server", eventID, state, "thread/status/changed"); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read codex app-server stream: %w", err)
	}
	return nil
}

// runOpenCodeEvent maps OpenCode plugin events to panefleet states.
// It accepts shape variations in payloads to stay resilient across plugin changes.
func runOpenCodeEvent(ctx context.Context, args []string) error {
	pane, _, skip, err := parsePaneOrSkip("opencode-event", args)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	payload, raw, eventID, ok, err := readLoggedStdinJSONPayload("opencode-event", pane)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	state := mapOpenCodeEvent(payload, strings.ToLower(string(raw)))
	if state == "" {
		logDecision("opencode-event", pane, eventID, "ignored", "", "event payload unmapped", "")
		return nil
	}

	return applyMappedState(ctx, pane, "opencode", "opencode-plugin", eventID, state, "plugin event mapped")
}

func notificationPayload(args []string) ([]byte, error) {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return []byte(args[0]), nil
	}
	return readAll(os.Stdin)
}

// parsePaneArgs centralizes pane resolution so all bridge entrypoints share
// the same precedence rules (flag, env, tmux pane).
func parsePaneArgs(command string, args []string) (string, []string, error) {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pane := fs.String("pane", defaultPane(), "tmux pane id")
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	return *pane, fs.Args(), nil
}

func parsePaneOrSkip(command string, args []string) (pane string, rest []string, skip bool, err error) {
	pane, rest, err = parsePaneArgs(command, args)
	if err != nil {
		return "", nil, false, err
	}
	if pane == "" {
		return "", rest, true, nil
	}
	return pane, rest, false, nil
}

func readLoggedStdinJSONPayload(source, pane string) (map[string]any, []byte, string, bool, error) {
	raw, err := readAll(os.Stdin)
	if err != nil {
		return nil, nil, "", false, err
	}
	payload, eventID, ok := decodeLoggedJSONPayload(source, pane, raw)
	return payload, raw, eventID, ok, nil
}

// decodeLoggedJSONPayload always emits a decision trail before mapping.
// This improves incident debugging when provider payloads drift.
func decodeLoggedJSONPayload(source, pane string, raw []byte) (map[string]any, string, bool) {
	eventID := nextEventID()
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, eventID, false
	}
	logPayload(source, pane, eventID, raw)

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		logDecision(source, pane, eventID, "decode_error", "", "invalid json payload", err.Error())
		return nil, eventID, false
	}
	return payload, eventID, true
}

// setState delegates writes to the panefleet CLI so bridge logic stays stateless
// and all persistence rules remain in a single place.
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

// applyMappedState records why a mapping was applied (or skipped) before returning.
// The extra logging is critical when users report "state stuck" issues.
func applyMappedState(ctx context.Context, pane, tool, source, eventID, status, reason string) error {
	if status == "" {
		logDecision(source, pane, eventID, "ignored", "", reason, "")
		return nil
	}
	if err := setState(ctx, pane, status, tool, source); err != nil {
		logDecision(source, pane, eventID, "state_set_error", status, reason, err.Error())
		return err
	}
	logDecision(source, pane, eventID, "state_set", status, reason, "")
	return nil
}

// runPanefleet applies a hard timeout to prevent provider hooks from hanging.
// Fast failure keeps editor/agent workflows responsive even when tmux is busy.
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

// defaultPane resolves the active pane from explicit bridge env first.
// This allows wrappers to target panes reliably outside the immediate shell context.
func defaultPane() string {
	if pane := os.Getenv("PANEFLEET_PANE"); pane != "" {
		return pane
	}
	return os.Getenv("TMUX_PANE")
}

// panefleetBin keeps a deterministic fallback path for source installs.
// Environment override is preferred so package-manager installs can inject paths.
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

// bridgeTimeout enforces bounded hook latency and accepts runtime override.
// Invalid overrides fail closed to the default timeout.
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

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
