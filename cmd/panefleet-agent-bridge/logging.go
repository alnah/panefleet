package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func nextEventID() string {
	return strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
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

// logPayload writes raw provider payloads for post-mortem debugging.
// Logging is opt-in to avoid persistent event data by default.
func logPayload(source, pane, eventID string, raw []byte) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return
	}

	record := struct {
		Timestamp string          `json:"ts"`
		Kind      string          `json:"kind"`
		EventID   string          `json:"event_id"`
		Source    string          `json:"source"`
		Pane      string          `json:"pane,omitempty"`
		Payload   json.RawMessage `json:"payload"`
	}{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Kind:      "payload",
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
