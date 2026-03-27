package main

import "strings"

// Payload helpers stay narrow on purpose: this bridge only needs lightweight
// JSON shape probing and substring checks, not a shared utility grab bag.

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
