package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alnah/panefleet/internal/panes"
	"github.com/alnah/panefleet/internal/state"
	"github.com/alnah/panefleet/internal/tmuxctl"
	"github.com/alnah/panefleet/internal/tmuxsync"
)

func cmdIngest(ctx context.Context, svc *panes.Service, args []string) error {
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
	if err := rejectUnexpectedArgs(fs); err != nil {
		return err
	}

	eventKind, err := parseKind(*kind)
	if err != nil {
		return err
	}
	occurredAt, err := parseOptionalRFC3339Time(*atRaw)
	if err != nil {
		return fmt.Errorf("invalid --at: %w", err)
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

func cmdStateShow(ctx context.Context, svc *panes.Service, args []string) error {
	fs := flag.NewFlagSet("state-show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pane := fs.String("pane", "", "tmux pane id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := rejectUnexpectedArgs(fs); err != nil {
		return err
	}
	out, err := svc.StateShow(ctx, *pane)
	if err != nil {
		return err
	}
	return printJSON(out)
}

func cmdStateList(ctx context.Context, svc *panes.Service, args []string) error {
	fs := flag.NewFlagSet("state-list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := rejectUnexpectedArgs(fs); err != nil {
		return err
	}
	out, err := svc.StateList(ctx)
	if err != nil {
		return err
	}
	return printJSON(out)
}

func cmdStateSet(ctx context.Context, svc *panes.Service, args []string) error {
	fs := flag.NewFlagSet("state-set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pane := fs.String("pane", "", "tmux pane id")
	statusRaw := fs.String("status", "", "override status")
	source := fs.String("source", "cli", "override source")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := rejectUnexpectedArgs(fs); err != nil {
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

func cmdStateClear(ctx context.Context, svc *panes.Service, args []string) error {
	fs := flag.NewFlagSet("state-clear", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pane := fs.String("pane", "", "tmux pane id")
	source := fs.String("source", "cli", "override source")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := rejectUnexpectedArgs(fs); err != nil {
		return err
	}
	out, err := svc.ClearOverride(ctx, *pane, *source)
	if err != nil {
		return err
	}
	return printJSON(out)
}

func cmdSyncTmux(ctx context.Context, svc *panes.Service, args []string) error {
	fs := flag.NewFlagSet("sync-tmux", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	atRaw := fs.String("at", "", "RFC3339 timestamp (default: now)")
	source := fs.String("source", "adapter:tmux-snapshot", "event source")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := rejectUnexpectedArgs(fs); err != nil {
		return err
	}

	at, err := parseOptionalRFC3339Time(*atRaw)
	if err != nil {
		return fmt.Errorf("invalid --at: %w", err)
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

func cmdPaneKill(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pane-kill", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pane := fs.String("pane", "", "tmux pane id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := rejectUnexpectedArgs(fs); err != nil {
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
	if err := rejectUnexpectedArgs(fs); err != nil {
		return err
	}
	tmux := tmuxctl.New(os.Getenv("PANEFLEET_TMUX_BIN"))
	if err := tmux.RespawnPane(ctx, *pane); err != nil {
		return err
	}
	return printJSON(map[string]any{"ok": true, "action": "respawn", "pane": *pane})
}

func syncTmuxOnce(ctx context.Context, svc *panes.Service, tmux *tmuxctl.ExecClient, at time.Time, source string) (int, error) {
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

func rejectUnexpectedArgs(fs *flag.FlagSet) error {
	if fs.NArg() == 0 {
		return nil
	}
	return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
}

func parseOptionalRFC3339Time(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Now().UTC(), nil
	}
	return time.Parse(time.RFC3339, raw)
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
