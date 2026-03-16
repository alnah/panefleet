package main

import "strings"

// mapClaudeHookEvent keeps Claude mapping conservative to avoid false positives.
// Unknown events are ignored so heuristic fallback can still decide state.
func mapClaudeHookEvent(event, lowerBlob string) (string, string) {
	switch {
	case containsString([]string{"PreToolUse", "UserPromptSubmit"}, event):
		return statusRun, "hook work event"
	case event == "PermissionRequest":
		return statusWait, "hook permission event"
	case event == "Stop":
		return statusDone, "hook completion event"
	case containsAny(lowerBlob, "error", "failed"):
		return statusError, "payload contains failure marker"
	default:
		return "", ""
	}
}

// mapCodexStatus translates Codex structured status into panefleet lifecycle states.
// It prefers explicit waiting flags over generic activity to avoid hiding approvals.
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

// mapOpenCodeEvent handles OpenCode event variants and infers a stable lifecycle.
// It intentionally prioritizes explicit error/permission signals over generic busy.
func mapOpenCodeEvent(payload map[string]any, lowerBlob string) string {
	eventType := openCodeEventType(payload)
	status := openCodeStatus(payload)

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
		return mapOpenCodeToolExecuteAfter(status, lowerBlob)
	case eventType == "permission.asked":
		return statusWait
	case eventType == "permission.replied":
		return mapOpenCodePermissionReply(payload)
	case containsAny(strings.ToLower(eventType), "error") || status == "error":
		return statusError
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
	status := stringValue(payload["status"])
	if status == "" {
		status = stringValue(event["status"])
	}
	return strings.ToLower(status)
}

func mapOpenCodeToolExecuteAfter(status, lowerBlob string) string {
	if containsAny(status, "error", "failed") || containsAny(lowerBlob, "\"error\"", "\"failed\"") {
		return statusError
	}
	return statusRun
}

func mapOpenCodePermissionReply(payload map[string]any) string {
	if permissionDecisionDenied(payload) {
		return statusError
	}
	if permissionDecisionApproved(payload) {
		return statusRun
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
