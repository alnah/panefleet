package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alnah/panefleet/internal/app"
	"github.com/alnah/panefleet/internal/state"
	"github.com/alnah/panefleet/internal/store"
	"github.com/alnah/panefleet/internal/tmuxctl"
	"github.com/alnah/panefleet/internal/tmuxsync"
	"github.com/alnah/panefleet/internal/tui"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "panefleet: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError()
	}

	svc, closer, err := newService(ctx)
	if err != nil {
		return err
	}
	defer closer()

	switch args[0] {
	case "ingest":
		return cmdIngest(ctx, svc, args[1:])
	case "state-show":
		return cmdStateShow(ctx, svc, args[1:])
	case "state-list":
		return cmdStateList(ctx, svc, args[1:])
	case "state-set":
		return cmdStateSet(ctx, svc, args[1:])
	case "state-clear":
		return cmdStateClear(ctx, svc, args[1:])
	case "sync-tmux":
		return cmdSyncTmux(ctx, svc, args[1:])
	case "pane-kill":
		return cmdPaneKill(ctx, args[1:])
	case "pane-respawn":
		return cmdPaneRespawn(ctx, args[1:])
	case "tui":
		return cmdTUI(svc, args[1:])
	case "run":
		return cmdRun(ctx, svc, args[1:])
	default:
		return usageError()
	}
}

func newService(ctx context.Context) (*app.Service, func(), error) {
	dbPath := os.Getenv("PANEFLEET_DB_PATH")
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, err
		}
		dbPath = filepath.Join(home, ".local", "state", "panefleet", "panefleet.db")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, nil, err
	}

	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		return nil, nil, err
	}
	if err := st.Init(ctx); err != nil {
		_ = st.Close()
		return nil, nil, err
	}
	if shouldHardenDBPermissions(dbPath) {
		if err := os.Chmod(dbPath, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = st.Close()
			return nil, nil, fmt.Errorf("secure db file permissions: %w", err)
		}
	}
	reducer, err := state.NewReducer(state.Config{
		DoneRecentWindow: 10 * time.Minute,
		StaleWindow:      45 * time.Minute,
	})
	if err != nil {
		_ = st.Close()
		return nil, nil, err
	}

	return app.NewService(reducer, st), func() { _ = st.Close() }, nil
}

func cmdIngest(ctx context.Context, svc *app.Service, args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pane := fs.String("pane", "", "tmux pane id (%12)")
	kind := fs.String("kind", "", "start|wait|exit|observe|override-set|override-clear|tick")
	exitCode := fs.Int("exit-code", 0, "exit code for kind=exit")
	override := fs.String("override", "", "status for kind=override-set")
	atRaw := fs.String("at", "", "RFC3339 timestamp (default: now)")
	source := fs.String("source", "cli", "event source")
	reason := fs.String("reason", "", "optional reason code")
	if err := fs.Parse(args); err != nil {
		return err
	}

	eventKind, err := parseKind(*kind)
	if err != nil {
		return err
	}
	occurredAt := time.Now().UTC()
	if *atRaw != "" {
		occurredAt, err = time.Parse(time.RFC3339, *atRaw)
		if err != nil {
			return fmt.Errorf("invalid --at: %w", err)
		}
	}

	ev := state.Event{
		PaneID:     *pane,
		Kind:       eventKind,
		OccurredAt: occurredAt,
		Source:     *source,
		ReasonCode: *reason,
	}
	if eventKind == state.EventPaneExited {
		code := *exitCode
		ev.ExitCode = &code
	}
	if eventKind == state.EventOverrideSet {
		parsed, err := state.ParseStatus(*override)
		if err != nil {
			return fmt.Errorf("--override: %w", err)
		}
		ev.OverrideTo = parsed
	}

	out, err := svc.Ingest(ctx, ev)
	if err != nil {
		return err
	}
	return printJSON(out)
}

func cmdStateShow(ctx context.Context, svc *app.Service, args []string) error {
	fs := flag.NewFlagSet("state-show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pane := fs.String("pane", "", "tmux pane id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	out, err := svc.StateShow(ctx, *pane)
	if err != nil {
		return err
	}
	return printJSON(out)
}

func cmdStateList(ctx context.Context, svc *app.Service, args []string) error {
	fs := flag.NewFlagSet("state-list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	out, err := svc.StateList(ctx)
	if err != nil {
		return err
	}
	return printJSON(out)
}

func cmdStateSet(ctx context.Context, svc *app.Service, args []string) error {
	fs := flag.NewFlagSet("state-set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pane := fs.String("pane", "", "tmux pane id")
	statusRaw := fs.String("status", "", "override status")
	source := fs.String("source", "cli", "override source")
	if err := fs.Parse(args); err != nil {
		return err
	}
	target, err := state.ParseStatus(*statusRaw)
	if err != nil {
		return err
	}
	out, err := svc.SetOverride(ctx, *pane, target, *source)
	if err != nil {
		return err
	}
	return printJSON(out)
}

func cmdStateClear(ctx context.Context, svc *app.Service, args []string) error {
	fs := flag.NewFlagSet("state-clear", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pane := fs.String("pane", "", "tmux pane id")
	source := fs.String("source", "cli", "override source")
	if err := fs.Parse(args); err != nil {
		return err
	}
	out, err := svc.ClearOverride(ctx, *pane, *source)
	if err != nil {
		return err
	}
	return printJSON(out)
}

func cmdTUI(svc *app.Service, args []string) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	refresh := fs.Duration("refresh", 700*time.Millisecond, "refresh interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := validatePositiveDuration("refresh", *refresh); err != nil {
		return err
	}
	tmux := tmuxctl.New(os.Getenv("PANEFLEET_TMUX_BIN"))
	updates, cancel := svc.Subscribe()
	defer cancel()
	api := runtimeAPI{svc: svc, tmux: tmux}
	m := tui.New(api, *refresh, updates)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func cmdSyncTmux(ctx context.Context, svc *app.Service, args []string) error {
	fs := flag.NewFlagSet("sync-tmux", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	atRaw := fs.String("at", "", "RFC3339 timestamp (default: now)")
	source := fs.String("source", "adapter:tmux-snapshot", "event source")
	if err := fs.Parse(args); err != nil {
		return err
	}

	at := time.Now().UTC()
	if *atRaw != "" {
		parsed, err := time.Parse(time.RFC3339, *atRaw)
		if err != nil {
			return fmt.Errorf("invalid --at: %w", err)
		}
		at = parsed
	}

	tmux := tmuxctl.New(os.Getenv("PANEFLEET_TMUX_BIN"))
	applied, err := syncTmuxOnce(ctx, svc, tmux, at, *source)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{
		"applied": applied,
		"source":  *source,
		"at":      at,
	})
}

func cmdRun(ctx context.Context, svc *app.Service, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	refresh := fs.Duration("refresh", 700*time.Millisecond, "tui refresh interval")
	syncEvery := fs.Duration("sync-every", 1200*time.Millisecond, "tmux sync interval")
	source := fs.String("source", "adapter:tmux-runner", "event source")
	controlMode := fs.Bool("control-mode", true, "enable tmux control-mode trigger stream")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := validatePositiveDuration("refresh", *refresh); err != nil {
		return err
	}
	if err := validatePositiveDuration("sync-every", *syncEvery); err != nil {
		return err
	}
	tmux := tmuxctl.New(os.Getenv("PANEFLEET_TMUX_BIN"))

	updates, cancel := svc.Subscribe()
	defer cancel()

	// Initial snapshot before opening the UI.
	_, _ = syncTmuxOnce(ctx, svc, tmux, time.Now().UTC(), *source)

	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	triggerSync := make(chan struct{}, 1)
	enqueueSync := func() {
		select {
		case triggerSync <- struct{}{}:
		default:
		}
	}
	if *controlMode {
		go func() {
			if err := tmux.WatchControlMode(runCtx, func(_ tmuxctl.ControlEvent) {
				enqueueSync()
			}); err != nil && runCtx.Err() == nil {
				fmt.Fprintf(os.Stderr, "panefleet: control-mode watcher disabled: %v\n", err)
			}
		}()
	}
	go func() {
		ticker := time.NewTicker(*syncEvery)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case t := <-ticker.C:
				_, _ = syncTmuxOnce(runCtx, svc, tmux, t.UTC(), *source)
			case <-triggerSync:
				_, _ = syncTmuxOnce(runCtx, svc, tmux, time.Now().UTC(), *source)
			}
		}
	}()

	api := runtimeAPI{svc: svc, tmux: tmux}
	model := tui.New(api, *refresh, updates)
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err := program.Run()
	return err
}

func cmdPaneKill(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pane-kill", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pane := fs.String("pane", "", "tmux pane id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tmux := tmuxctl.New(os.Getenv("PANEFLEET_TMUX_BIN"))
	if err := tmux.KillPane(ctx, *pane); err != nil {
		return err
	}
	return printJSON(map[string]any{"ok": true, "action": "kill", "pane": *pane})
}

func cmdPaneRespawn(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pane-respawn", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pane := fs.String("pane", "", "tmux pane id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tmux := tmuxctl.New(os.Getenv("PANEFLEET_TMUX_BIN"))
	if err := tmux.RespawnPane(ctx, *pane); err != nil {
		return err
	}
	return printJSON(map[string]any{"ok": true, "action": "respawn", "pane": *pane})
}

func syncTmuxOnce(ctx context.Context, svc *app.Service, tmux *tmuxctl.ExecClient, at time.Time, source string) (int, error) {
	snapshot, err := tmux.Snapshot(ctx)
	if err != nil {
		return 0, err
	}
	events := tmuxsync.EventsFromSnapshot(snapshot, at, source)
	applied := 0
	for _, ev := range events {
		if _, err := svc.Ingest(ctx, ev); err != nil {
			return applied, fmt.Errorf("ingest pane %s: %w", ev.PaneID, err)
		}
		applied++
	}
	return applied, nil
}

func validatePositiveDuration(name string, value time.Duration) error {
	if value <= 0 {
		return fmt.Errorf("%s must be > 0", name)
	}
	return nil
}

func shouldHardenDBPermissions(dbPath string) bool {
	if dbPath == "" || dbPath == ":memory:" {
		return false
	}
	if strings.HasPrefix(dbPath, "file:") {
		return false
	}
	if strings.Contains(dbPath, "?") {
		return false
	}
	return true
}

type runtimeAPI struct {
	svc  *app.Service
	tmux *tmuxctl.ExecClient
}

func (r runtimeAPI) StateList(ctx context.Context) ([]state.PaneState, error) {
	return r.svc.StateList(ctx)
}

func (r runtimeAPI) SetOverride(ctx context.Context, paneID string, target state.Status, source string) (state.PaneState, error) {
	return r.svc.SetOverride(ctx, paneID, target, source)
}

func (r runtimeAPI) ClearOverride(ctx context.Context, paneID, source string) (state.PaneState, error) {
	return r.svc.ClearOverride(ctx, paneID, source)
}

func (r runtimeAPI) KillPane(ctx context.Context, paneID string) error {
	return r.tmux.KillPane(ctx, paneID)
}

func (r runtimeAPI) RespawnPane(ctx context.Context, paneID string) error {
	return r.tmux.RespawnPane(ctx, paneID)
}

func parseKind(raw string) (state.EventKind, error) {
	switch raw {
	case "start":
		return state.EventPaneStarted, nil
	case "wait":
		return state.EventPaneWaiting, nil
	case "exit":
		return state.EventPaneExited, nil
	case "observe":
		return state.EventPaneObserved, nil
	case "override-set":
		return state.EventOverrideSet, nil
	case "override-clear":
		return state.EventOverrideCleared, nil
	case "tick":
		return state.EventTimerRecompute, nil
	default:
		return "", fmt.Errorf("unsupported --kind: %s", raw)
	}
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func usageError() error {
	return errors.New("usage: panefleet <ingest|state-show|state-list|state-set|state-clear|sync-tmux|pane-kill|pane-respawn|tui|run> [flags]")
}
