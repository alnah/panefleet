package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	statepkg "github.com/alnah/panefleet/internal/state"
)

// runClaudeHook maps Claude hook payloads to panefleet states.
// It is intentionally tolerant of missing/partial payloads so Claude flows are
// not blocked when hooks misfire or emit unknown events.
func runClaudeHook(ctx context.Context, args []string) error {
	pane, rest, skip, err := parsePaneOrSkip("claude-hook", args)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}
	if err := rejectBridgeUnexpectedArgs(rest); err != nil {
		return err
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

	mappedStatus, reason := mapClaudeHookEvent(event, strings.ToLower(string(raw)))
	if mappedStatus == "" {
		logDecision("claude-hook", pane, eventID, "ignored", "", "unmapped hook event", "")
		return nil
	}
	if err := applyMappedState(ctx, pane, "claude-hook", eventID, mappedStatus, reason); err != nil {
		return err
	}
	if mappedStatus == statepkg.StatusDone {
		return applyClaudeTranscriptMetrics(ctx, pane, "claude-hook", eventID, payload)
	}
	return nil
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
		if err := applyMappedState(ctx, pane, "codex-notify", eventID, statepkg.StatusDone, "notify agent-turn-complete"); err != nil {
			return err
		}
		return applyCodexNotifyMetrics(ctx, pane, "codex-notify", eventID, payload)
	}
	logDecision("codex-notify", pane, eventID, "ignored", "", "notify payload type not mapped", "")
	return nil
}

// runCodexAppServer consumes Codex status-change stream events and updates state.
// Non-status events are logged and ignored to keep the state machine stable.
func runCodexAppServer(ctx context.Context, args []string) error {
	pane, rest, skip, err := parsePaneOrSkip("codex-app-server", args)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}
	if err := rejectBridgeUnexpectedArgs(rest); err != nil {
		return err
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
		method := stringValue(payload["method"])
		switch method {
		case "thread/status/changed":
			state := mapCodexStatus(payload)
			if state == "" {
				logDecision("codex-app-server", pane, eventID, "ignored", "", "status payload unmapped", "")
				continue
			}
			if err := applyMappedState(ctx, pane, "codex-app-server", eventID, state, "thread/status/changed"); err != nil {
				return err
			}
		case "thread/tokenUsage/updated":
			if err := applyCodexTokenUsage(ctx, pane, "codex-app-server", eventID, payload); err != nil {
				return err
			}
		default:
			logDecision("codex-app-server", pane, eventID, "ignored", "", "unsupported app-server method", method)
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
	pane, rest, skip, err := parsePaneOrSkip("opencode-event", args)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}
	if err := rejectBridgeUnexpectedArgs(rest); err != nil {
		return err
	}

	payload, raw, eventID, ok, err := readLoggedStdinJSONPayload("opencode-event", pane)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	state := mapOpenCodeEvent(payload, strings.ToLower(string(raw)))
	if err := applyOpenCodeUsageMetrics(ctx, pane, "opencode-event", eventID, payload); err != nil {
		return err
	}
	if state == "" {
		logDecision("opencode-event", pane, eventID, "ignored", "", "event payload unmapped", "")
		return nil
	}

	return applyMappedState(ctx, pane, "opencode-plugin", eventID, state, "plugin event mapped")
}
