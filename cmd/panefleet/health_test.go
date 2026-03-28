package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectHealthReportRejectsUnknownCheck(t *testing.T) {
	if _, err := collectHealthReport(context.Background(), "nope"); err == nil {
		t.Fatalf("expected unknown check to fail")
	}
}

func TestCollectHealthReportLivenessOKWithoutTMUXSession(t *testing.T) {
	t.Setenv("PANEFLEET_DB_PATH", filepath.Join(t.TempDir(), "panefleet.db"))
	t.Setenv("TMUX", "")

	report, err := collectHealthReport(context.Background(), "liveness")
	if err != nil {
		t.Fatalf("collectHealthReport(liveness): %v", err)
	}
	if report.Status != "ok" || !report.Live || !report.Ready {
		t.Fatalf("unexpected liveness report: %+v", report)
	}
	if report.Config.DBPath == "" {
		t.Fatalf("expected db path in config")
	}
}

func TestCollectHealthReportReadinessRequiresTMUXSession(t *testing.T) {
	t.Setenv("PANEFLEET_DB_PATH", filepath.Join(t.TempDir(), "panefleet.db"))
	t.Setenv("TMUX", "")

	report, err := collectHealthReport(context.Background(), "readiness")
	if err != nil {
		t.Fatalf("collectHealthReport(readiness): %v", err)
	}
	if report.Live != true {
		t.Fatalf("readiness should keep liveness true when only tmux session is missing: %+v", report)
	}
	if report.Ready {
		t.Fatalf("readiness should be false without TMUX session: %+v", report)
	}
	if report.Status != "fail" {
		t.Fatalf("status = %q, want fail", report.Status)
	}
}

func TestCollectHealthReportReadinessChecksTMUXSnapshot(t *testing.T) {
	t.Setenv("PANEFLEET_DB_PATH", filepath.Join(t.TempDir(), "panefleet.db"))
	t.Setenv("TMUX", "1")
	t.Setenv("PANEFLEET_TMUX_BIN", writeFakeTmux(t))

	report, err := collectHealthReport(context.Background(), "readiness")
	if err != nil {
		t.Fatalf("collectHealthReport(readiness): %v", err)
	}
	if !report.Live || !report.Ready || report.Status != "ok" {
		t.Fatalf("unexpected readiness report: %+v", report)
	}
	if len(report.Checks) == 0 {
		t.Fatalf("expected checks in readiness report")
	}
}

func TestCmdHealthPrintsJSONAndFailsOnUnreadyReadiness(t *testing.T) {
	t.Setenv("PANEFLEET_DB_PATH", filepath.Join(t.TempDir(), "panefleet.db"))
	t.Setenv("TMUX", "")

	output := captureStdout(t, func() {
		err := cmdHealth(context.Background(), []string{"--check", "readiness"})
		if err == nil || !strings.Contains(err.Error(), "readiness health failed") {
			t.Fatalf("expected readiness failure, got %v", err)
		}
	})

	var report healthReport
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("health output should be valid json: %v\n%s", err, output)
	}
	if report.Check != "readiness" || report.Status != "fail" {
		t.Fatalf("unexpected report: %+v", report)
	}
}
