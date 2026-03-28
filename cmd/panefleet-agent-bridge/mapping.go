package main

import (
	"strings"

	"github.com/alnah/panefleet/internal/state"
)

// mapClaudeHookEvent keeps Claude mapping conservative to avoid false positives.
// Unknown events are ignored so heuristic fallback can still decide state.
func mapClaudeHookEvent(event, lowerBlob string) (state.Status, string) {
	switch {
	case containsString([]string{"PreToolUse", "UserPromptSubmit"}, event):
		return state.StatusRun, "hook work event"
	case event == "PermissionRequest":
		return state.StatusWait, "hook permission event"
	case event == "Stop":
		return state.StatusDone, "hook completion event"
	case containsAny(lowerBlob, "error", "failed"):
		return state.StatusError, "payload contains failure marker"
	default:
		return "", ""
	}
}

// mapCodexStatus translates Codex structured status into panefleet lifecycle states.
// It prefers explicit waiting flags over generic activity to avoid hiding approvals.
func mapCodexStatus(payload map[string]any) state.Status {
	params := mapValue(payload["params"])
	status := mapValue(params["status"])
	statusType := stringValue(status["type"])

	switch statusType {
	case "active":
		if boolValue(status["waitingOnApproval"]) || containsString(activeFlags(status), "waitingOnApproval") {
			return state.StatusWait
		}
		return state.StatusRun
	case "idle":
		return state.StatusDone
	case "systemError":
		return state.StatusError
	default:
		return ""
	}
}

// mapOpenCodeEvent handles OpenCode event variants and infers a stable lifecycle.
// It intentionally prioritizes explicit error/permission signals over generic busy.
func mapOpenCodeEvent(payload map[string]any, lowerBlob string) state.Status {
	eventType := openCodeEventType(payload)
	status := openCodeStatus(payload)

	switch {
	case eventType == "session.idle":
		return state.StatusDone
	case eventType == "session.error":
		return state.StatusError
	case eventType == "session.status" && containsString([]string{"busy", "running", "active"}, status):
		return state.StatusRun
	case eventType == "message.part.delta":
		return state.StatusRun
	case strings.HasPrefix(eventType, "tool.execute.before"):
		return state.StatusRun
	case strings.HasPrefix(eventType, "tool.execute.after"):
		return mapOpenCodeToolExecuteAfter(status, lowerBlob)
	case eventType == "permission.asked":
		return state.StatusWait
	case eventType == "permission.replied":
		return mapOpenCodePermissionReply(payload)
	case containsAny(strings.ToLower(eventType), "error") || status == "error":
		return state.StatusError
	default:
		return ""
	}
}

func openCodeEventType(payload map[string]any) string {
	event := mapValue(payload["event"])
	eventType := stringValue(payload["type"])
	if eventType == "" {
		eventType = stringValue(event["type"])
	}
	return eventType
}

func openCodeStatus(payload map[string]any) string {
	event := mapValue(payload["event"])
	properties := mapValue(event["properties"])
	nestedStatus := mapValue(properties["status"])
	status := stringValue(payload["status"])
	if status == "" {
		status = stringValue(event["status"])
	}
	if status == "" {
		status = stringValue(nestedStatus["type"])
	}
	return strings.ToLower(status)
}

func mapOpenCodeToolExecuteAfter(status, lowerBlob string) state.Status {
	if containsAny(status, "error", "failed") || containsAny(lowerBlob, "\"error\"", "\"failed\"") {
		return state.StatusError
	}
	return state.StatusRun
}

func mapOpenCodePermissionReply(payload map[string]any) state.Status {
	if permissionDecisionDenied(payload) {
		return state.StatusError
	}
	if permissionDecisionApproved(payload) {
		return state.StatusRun
	}
	return ""
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
