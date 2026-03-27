package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var bridgeRunID = nextEventID()
var eventSeq atomic.Uint64

func nextEventID() string {
	seq := eventSeq.Add(1)
	return fmt.Sprintf("%s-%s", strconv.FormatInt(time.Now().UTC().UnixNano(), 36), strconv.FormatUint(seq, 36))
}

func eventLogDir() string {
	return os.Getenv("PANEFLEET_EVENT_LOG_DIR")
}

func ensureEventLogDir(logDir string) bool {
	if logDir == "" {
		return false
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return false
	}
	// #nosec G302 -- event log directories need execute permission; 0700 keeps them private to the current user.
	if err := os.Chmod(logDir, 0o700); err != nil {
		return false
	}
	return true
}

func appendJSONLogRecord(source string, record any) {
	logDir := eventLogDir()
	if !ensureEventLogDir(logDir) {
		return
	}
	logPath := eventLogPath(source)
	if logPath == "" {
		return
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		return
	}

	root, name, err := openEventLogRoot(logDir, source)
	if err != nil {
		return
	}
	defer func() { _ = root.Close() }()

	file, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	if err := file.Chmod(0o600); err != nil {
		return
	}
	_, _ = file.Write(append(encoded, '\n'))
}

func eventLogPath(source string) string {
	name := sanitizeLogSource(source)
	if name == "" {
		return ""
	}
	return name + ".jsonl"
}

func openEventLogRoot(logDir, source string) (*os.Root, string, error) {
	name := eventLogPath(source)
	if name == "" {
		return nil, "", fmt.Errorf("event log source is required")
	}
	root, err := os.OpenRoot(logDir)
	if err != nil {
		return nil, "", err
	}
	return root, name, nil
}

func sanitizeLogSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range source {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	name := strings.Trim(b.String(), "._-")
	if name == "" {
		return ""
	}
	return name
}

// logPayload writes raw provider payloads for post-mortem debugging.
// Logging is opt-in to avoid persistent event data by default.
func logPayload(source, pane, eventID string, raw []byte) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return
	}

	record := struct {
		Timestamp string          `json:"ts"`
		Kind      string          `json:"kind"`
		RunID     string          `json:"run_id"`
		EventID   string          `json:"event_id"`
		Source    string          `json:"source"`
		Pane      string          `json:"pane,omitempty"`
		Payload   json.RawMessage `json:"payload"`
	}{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Kind:      "payload",
		RunID:     bridgeRunID,
		EventID:   eventID,
		Source:    source,
		Pane:      pane,
		Payload:   json.RawMessage(bytes.TrimSpace(raw)),
	}

	appendJSONLogRecord(source, record)
}

// logDecision records the bridge decision path next to payload logs.
// Keeping decision and payload in the same stream helps diff mapping regressions.
func logDecision(source, pane, eventID, decision, status, reason, errText string) {
	record := struct {
		Timestamp string `json:"ts"`
		Kind      string `json:"kind"`
		RunID     string `json:"run_id"`
		EventID   string `json:"event_id"`
		Source    string `json:"source"`
		Pane      string `json:"pane,omitempty"`
		Decision  string `json:"decision"`
		Status    string `json:"status,omitempty"`
		Reason    string `json:"reason,omitempty"`
		Error     string `json:"error,omitempty"`
	}{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Kind:      "decision",
		RunID:     bridgeRunID,
		EventID:   eventID,
		Source:    source,
		Pane:      pane,
		Decision:  decision,
		Status:    status,
		Reason:    reason,
		Error:     errText,
	}

	appendJSONLogRecord(source, record)
}
