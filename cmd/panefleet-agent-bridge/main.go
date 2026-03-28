package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// panefleet-agent-bridge converts provider-specific events into panefleet states.
// It exists to keep tmux-side heuristics simple while allowing explicit state
// updates when providers expose machine-readable events.

const (
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

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
