package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alnah/panefleet/internal/app"
	"github.com/alnah/panefleet/internal/state"
	"github.com/alnah/panefleet/internal/store"
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
	case "sync-tmux":
		return cmdSyncTmux(ctx, svc, args[1:])
	case "tui":
		return cmdTUI(svc, args[1:])
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
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
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

func cmdTUI(svc *app.Service, args []string) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	refresh := fs.Duration("refresh", 700*time.Millisecond, "refresh interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	m := tui.New(svc, *refresh)
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

	cmd := exec.CommandContext(ctx, "tmux", "list-panes", "-a", "-F", tmuxsync.ListPanesFormat)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("tmux list-panes failed: %w", err)
	}
	snapshot, err := tmuxsync.ParseListPanesOutput(string(out))
	if err != nil {
		return err
	}
	events := tmuxsync.EventsFromSnapshot(snapshot, at, *source)

	applied := 0
	for _, ev := range events {
		if _, err := svc.Ingest(ctx, ev); err != nil {
			return fmt.Errorf("ingest pane %s: %w", ev.PaneID, err)
		}
		applied++
	}
	return printJSON(map[string]any{
		"applied": applied,
		"source":  *source,
		"at":      at,
	})
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
	return errors.New("usage: panefleet <ingest|state-show|state-list|sync-tmux|tui> [flags]")
}
