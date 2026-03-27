package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// applyMappedState records why a mapping was applied (or skipped) before returning.
// The extra logging is critical when users report "state stuck" issues.
func applyMappedState(ctx context.Context, pane, source, eventID, status, reason string) error {
	if status == "" {
		logDecision(source, pane, eventID, "ignored", "", reason, "")
		return nil
	}
	if err := ingestState(ctx, pane, status, source); err != nil {
		logDecision(source, pane, eventID, "ingest_error", status, reason, err.Error())
		return err
	}
	logDecision(source, pane, eventID, "ingest", status, reason, "")
	return nil
}

// ingestState maps provider lifecycle states onto panefleet ingest events so
// bridge traffic updates the underlying stream instead of forcing overrides.
func ingestState(ctx context.Context, pane, status, source string) error {
	args := []string{
		"ingest",
		"--pane", pane,
		"--source", source,
	}
	switch status {
	case statusRun:
		args = append(args, "--kind", "start")
	case statusWait:
		args = append(args, "--kind", "wait")
	case statusDone:
		args = append(args, "--kind", "exit", "--exit-code", "0")
	case statusError:
		args = append(args, "--kind", "exit", "--exit-code", "1")
	default:
		return fmt.Errorf("unsupported mapped status: %s", status)
	}
	return runPanefleet(ctx, args...)
}

// runPanefleet applies a hard timeout to prevent provider hooks from hanging.
// Fast failure keeps editor/agent workflows responsive even when tmux is busy.
func runPanefleet(ctx context.Context, args ...string) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, bridgeTimeout())
	defer cancel()

	// #nosec G204,G702 -- the bridge intentionally executes the configured panefleet binary with fixed subcommands assembled in code.
	cmd := exec.CommandContext(timeoutCtx, panefleetBin(), args...)
	cmd.Stdout = nil
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("run panefleet %s: timeout after %s", strings.Join(args, " "), bridgeTimeout())
		}
		errText := strings.TrimSpace(stderr.String())
		if errText != "" {
			return fmt.Errorf("run panefleet %s: %w: %s", strings.Join(args, " "), err, errText)
		}
		return fmt.Errorf("run panefleet %s: %w", strings.Join(args, " "), err)
	}
	return nil
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
