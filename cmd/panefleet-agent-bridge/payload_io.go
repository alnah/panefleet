package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func notificationPayload(args []string) ([]byte, error) {
	if len(args) > 1 {
		return nil, fmt.Errorf("unexpected arguments: %s", strings.Join(args[1:], " "))
	}
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
		logDecision(command, "", nextEventID(), "ignored", "", "pane unresolved", "set --pane or PANEFLEET_PANE")
		return "", rest, true, nil
	}
	return pane, rest, false, nil
}

func rejectBridgeUnexpectedArgs(args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
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
