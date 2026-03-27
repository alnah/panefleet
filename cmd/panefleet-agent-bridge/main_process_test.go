package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const bridgeArgsSep = "\n"

func TestBridgeMainProcessUsageExit(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestBridgeMainHelperProcess")
	cmd.Env = append(os.Environ(),
		"GO_WANT_BRIDGE_HELPER=1",
		"BRIDGE_ARGS="+strings.Join([]string{"panefleet-agent-bridge"}, bridgeArgsSep),
	)
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit for missing args")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit code, got %v", err)
	}
}

func TestBridgeMainProcessCodexNotifyPath(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestBridgeMainHelperProcess")
	cmd.Env = append(os.Environ(),
		"GO_WANT_BRIDGE_HELPER=1",
		"PANEFLEET_PANE=%3",
		"BRIDGE_STDIN={\"type\":\"noop\"}",
		"BRIDGE_ARGS="+strings.Join([]string{"panefleet-agent-bridge", "codex-notify"}, bridgeArgsSep),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("expected success for codex-notify noop, err=%v out=%s", err, string(out))
	}
}

func TestBridgeMainHelperProcess(_ *testing.T) {
	if os.Getenv("GO_WANT_BRIDGE_HELPER") != "1" {
		return
	}
	args := strings.Split(os.Getenv("BRIDGE_ARGS"), bridgeArgsSep)
	if len(args) == 0 {
		os.Exit(1)
	}
	os.Args = args
	if payload := os.Getenv("BRIDGE_STDIN"); payload != "" {
		r, w, err := os.Pipe()
		if err != nil {
			os.Exit(1)
		}
		_, _ = w.WriteString(payload)
		_ = w.Close()
		old := os.Stdin
		os.Stdin = r
		defer func() { os.Stdin = old }()
	}
	main()
	os.Exit(0)
}
