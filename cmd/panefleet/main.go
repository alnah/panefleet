package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alnah/panefleet/internal/panes"
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

	switch args[0] {
	case "ingest":
		return runWithService(ctx, args[1:], cmdIngest)
	case "state-show":
		return runWithService(ctx, args[1:], cmdStateShow)
	case "state-list":
		return runWithService(ctx, args[1:], cmdStateList)
	case "state-set":
		return runWithService(ctx, args[1:], cmdStateSet)
	case "state-clear":
		return runWithService(ctx, args[1:], cmdStateClear)
	case "sync-tmux":
		return runWithService(ctx, args[1:], cmdSyncTmux)
	case "health":
		return cmdHealth(ctx, args[1:])
	case "pane-kill":
		return cmdPaneKill(ctx, args[1:])
	case "pane-respawn":
		return cmdPaneRespawn(ctx, args[1:])
	case "tui":
		return runWithService(ctx, args[1:], func(_ context.Context, svc *panes.Service, args []string) error {
			return cmdTUI(svc, args)
		})
	case "run":
		return runWithService(ctx, args[1:], cmdRun)
	default:
		return usageError()
	}
}

type serviceCommand func(context.Context, *panes.Service, []string) error

func runWithService(ctx context.Context, args []string, run serviceCommand) error {
	svc, closer, err := newService(ctx)
	if err != nil {
		return err
	}
	defer closer()
	return run(ctx, svc, args)
}
