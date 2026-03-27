package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const mainArgsSep = "\n"

func TestMainProcessUsageExit(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
	cmd.Env = append(os.Environ(),
		"GO_WANT_MAIN_HELPER=1",
		"MAIN_ARGS="+strings.Join([]string{"panefleet"}, mainArgsSep),
		"PANEFLEET_DB_PATH="+filepath.Join(t.TempDir(), "panefleet.db"),
	)
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit for missing command")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit code, got %v", err)
	}
}

func TestMainProcessStateListExitZero(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "panefleet.db")
	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
	cmd.Env = append(os.Environ(),
		"GO_WANT_MAIN_HELPER=1",
		"PANEFLEET_DB_PATH="+dbPath,
		"MAIN_ARGS="+strings.Join([]string{"panefleet", "state-list"}, mainArgsSep),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("expected zero exit for state-list, err=%v out=%s", err, string(out))
	}
}

func TestMainHelperProcess(_ *testing.T) {
	if os.Getenv("GO_WANT_MAIN_HELPER") != "1" {
		return
	}
	args := strings.Split(os.Getenv("MAIN_ARGS"), mainArgsSep)
	if len(args) == 0 {
		os.Exit(1)
	}
	os.Args = args
	main()
	os.Exit(0)
}
