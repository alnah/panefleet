package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alnah/panefleet/internal/tmuxctl"
	_ "modernc.org/sqlite"
)

type healthCheckResult struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Detail   string `json:"detail,omitempty"`
	Required bool   `json:"required"`
}

type healthConfig struct {
	DBPath      string `json:"db_path,omitempty"`
	TmuxBin     string `json:"tmux_bin,omitempty"`
	TmuxSession bool   `json:"tmux_session"`
}

type healthReport struct {
	Check     string              `json:"check"`
	Status    string              `json:"status"`
	Live      bool                `json:"live"`
	Ready     bool                `json:"ready"`
	Timestamp time.Time           `json:"timestamp"`
	Config    healthConfig        `json:"config"`
	Checks    []healthCheckResult `json:"checks"`
}

func cmdHealth(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	check := fs.String("check", "readiness", "liveness|readiness")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := rejectUnexpectedArgs(fs); err != nil {
		return err
	}

	report, err := collectHealthReport(ctx, *check)
	if err != nil {
		return err
	}
	if err := printJSON(report); err != nil {
		return err
	}
	if report.Status != "ok" {
		return fmt.Errorf("%s health failed", report.Check)
	}
	return nil
}

func collectHealthReport(ctx context.Context, check string) (healthReport, error) {
	switch check {
	case "liveness", "readiness":
	default:
		return healthReport{}, fmt.Errorf("unsupported --check: %s", check)
	}

	report := healthReport{
		Check:     check,
		Status:    "ok",
		Live:      true,
		Ready:     check == "liveness",
		Timestamp: time.Now().UTC(),
		Config: healthConfig{
			TmuxBin:     resolvedTMUXBin(),
			TmuxSession: strings.TrimSpace(os.Getenv("TMUX")) != "",
		},
	}

	dbPath, err := resolveDBPath()
	if err != nil {
		report.Live = false
		report.Ready = false
		report.Status = "fail"
		report.Checks = append(report.Checks, healthCheckResult{
			Name:     "db.path.resolve",
			OK:       false,
			Detail:   err.Error(),
			Required: true,
		})
		return report, nil
	}
	report.Config.DBPath = dbPath
	report.Checks = append(report.Checks, healthCheckResult{
		Name:     "db.path.resolve",
		OK:       true,
		Detail:   dbPath,
		Required: true,
	})

	dbChecks := dbHealthChecks(ctx, dbPath)
	report.Checks = append(report.Checks, dbChecks...)
	for _, result := range dbChecks {
		if result.Required && !result.OK {
			report.Live = false
			report.Ready = false
			report.Status = "fail"
		}
	}

	if check == "readiness" {
		readinessChecks := tmuxReadinessChecks(ctx, report.Config.TmuxBin, report.Config.TmuxSession)
		report.Checks = append(report.Checks, readinessChecks...)
		report.Ready = report.Live
		for _, result := range readinessChecks {
			if result.Required && !result.OK {
				report.Ready = false
				report.Status = "fail"
			}
		}
	}

	return report, nil
}

func dbHealthChecks(ctx context.Context, dbPath string) []healthCheckResult {
	if !shouldManageDBPath(dbPath) {
		return []healthCheckResult{{
			Name:     "db.path.mode",
			OK:       true,
			Detail:   "non-file sqlite path",
			Required: true,
		}}
	}

	dir := filepath.Dir(dbPath)
	results := []healthCheckResult{{
		Name:     "db.dir.exists",
		OK:       true,
		Detail:   dir,
		Required: true,
	}}
	if info, err := os.Stat(dir); err != nil {
		results[0].OK = false
		results[0].Detail = err.Error()
		return results
	} else if !info.IsDir() {
		results[0].OK = false
		results[0].Detail = "not a directory"
		return results
	}

	info, err := os.Stat(dbPath)
	switch {
	case err == nil:
		results = append(results, healthCheckResult{
			Name:     "db.file.exists",
			OK:       true,
			Detail:   dbPath,
			Required: true,
		})
	case os.IsNotExist(err):
		results = append(results, healthCheckResult{
			Name:     "db.file.exists",
			OK:       true,
			Detail:   "db file missing; first write will create it",
			Required: true,
		})
		return results
	default:
		results = append(results, healthCheckResult{
			Name:     "db.file.exists",
			OK:       false,
			Detail:   err.Error(),
			Required: true,
		})
		return results
	}

	if info.IsDir() {
		results = append(results, healthCheckResult{
			Name:     "db.file.open",
			OK:       false,
			Detail:   "db path is a directory",
			Required: true,
		})
		return results
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		results = append(results, healthCheckResult{
			Name:     "db.file.open",
			OK:       false,
			Detail:   err.Error(),
			Required: true,
		})
		return results
	}
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		results = append(results, healthCheckResult{
			Name:     "db.file.open",
			OK:       false,
			Detail:   err.Error(),
			Required: true,
		})
		return results
	}
	results = append(results, healthCheckResult{
		Name:     "db.file.open",
		OK:       true,
		Detail:   "sqlite ping ok",
		Required: true,
	})
	return results
}

func tmuxReadinessChecks(ctx context.Context, tmuxBin string, hasSession bool) []healthCheckResult {
	results := []healthCheckResult{{
		Name:     "tmux.bin.lookup",
		OK:       true,
		Detail:   tmuxBin,
		Required: true,
	}}
	if _, err := exec.LookPath(tmuxBin); err != nil {
		results[0].OK = false
		results[0].Detail = err.Error()
		return results
	}

	results = append(results, healthCheckResult{
		Name:     "tmux.session",
		OK:       hasSession,
		Detail:   "TMUX environment present",
		Required: true,
	})
	if !hasSession {
		return results
	}

	tmux := tmuxctl.New(tmuxBin)
	snapCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	snapshot, err := tmux.Snapshot(snapCtx)
	if err != nil {
		results = append(results, healthCheckResult{
			Name:     "tmux.snapshot",
			OK:       false,
			Detail:   err.Error(),
			Required: true,
		})
		return results
	}
	results = append(results, healthCheckResult{
		Name:     "tmux.snapshot",
		OK:       true,
		Detail:   fmt.Sprintf("%d panes", len(snapshot)),
		Required: true,
	})
	return results
}

func resolvedTMUXBin() string {
	raw := strings.TrimSpace(os.Getenv("PANEFLEET_TMUX_BIN"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("TMUX_BIN"))
	}
	if raw == "" {
		raw = "tmux"
	}
	return raw
}
