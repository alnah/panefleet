package main

import (
	"math"
	"strings"
)

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

func int64Value(v any) (int64, bool) {
	switch value := v.(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case int32:
		return int64(value), true
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, false
		}
		return int64(value), true
	default:
		return 0, false
	}
}
