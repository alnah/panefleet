package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alnah/panefleet/internal/board"
	"github.com/alnah/panefleet/internal/panes"
	"github.com/alnah/panefleet/internal/state"
	"github.com/alnah/panefleet/internal/tmuxctl"
	"github.com/alnah/panefleet/internal/tui"
)

type teaProgram interface {
	Run() (tea.Model, error)
}

var newTeaProgram = func(model tea.Model, opts ...tea.ProgramOption) teaProgram {
	return tea.NewProgram(model, opts...)
}

func cmdTUI(svc *panes.Service, args []string) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	refresh := fs.Duration("refresh", 700*time.Millisecond, "refresh interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := rejectUnexpectedArgs(fs); err != nil {
		return err
	}
	if err := validatePositiveDuration("refresh", *refresh); err != nil {
		return err
	}
	if err := requireTMUXSession(); err != nil {
		return err
	}
	tmux := tmuxctl.New(os.Getenv("PANEFLEET_TMUX_BIN"))
	if _, err := syncTmuxOnce(context.Background(), svc, tmux, time.Now().UTC(), "adapter:tmux-tui"); err != nil {
		fmt.Fprintf(os.Stderr, "panefleet: initial tmux sync failed: %v\n", err)
	}
	boardRuntime := board.NewService(svc, tmux, os.Getenv("TMUX_PANE"))
	m := tui.NewBoard(boardRuntime, *refresh)
	p := newTeaProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func cmdRun(ctx context.Context, svc *panes.Service, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	refresh := fs.Duration("refresh", 700*time.Millisecond, "tui refresh interval")
	syncEvery := fs.Duration("sync-every", 1200*time.Millisecond, "tmux sync interval")
	source := fs.String("source", "adapter:tmux-runner", "event source")
	controlMode := fs.Bool("control-mode", true, "enable tmux control-mode trigger stream")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := rejectUnexpectedArgs(fs); err != nil {
		return err
	}
	if err := validatePositiveDuration("refresh", *refresh); err != nil {
		return err
	}
	if err := validatePositiveDuration("sync-every", *syncEvery); err != nil {
		return err
	}
	if err := requireTMUXSession(); err != nil {
		return err
	}
	tmux := tmuxctl.New(os.Getenv("PANEFLEET_TMUX_BIN"))
	runID := newRunID()

	appliedInitial, err := syncTmuxOnce(ctx, svc, tmux, time.Now().UTC(), *source)
	if err != nil {
		runLogf(runID, "sync.initial source=%s err=%v", *source, err)
	} else if runVerboseEnabled() {
		runLogf(runID, "sync.initial source=%s applied=%d", *source, appliedInitial)
	}

	runCtx, stop := context.WithCancel(ctx)
	triggerSync := make(chan struct{}, 1)
	enqueueSync := func() {
		select {
		case triggerSync <- struct{}{}:
		default:
		}
	}
	var bg sync.WaitGroup
	if *controlMode {
		bg.Add(1)
		go func() {
			defer bg.Done()
			if err := tmux.WatchControlMode(runCtx, func(_ tmuxctl.ControlEvent) {
				enqueueSync()
			}); err != nil && runCtx.Err() == nil {
				runLogf(runID, "watch.control_mode status=disabled err=%v", err)
			}
		}()
	}
	bg.Add(1)
	go func() {
		defer bg.Done()
		ticker := time.NewTicker(*syncEvery)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case t := <-ticker.C:
				applied, err := syncTmuxOnce(runCtx, svc, tmux, t.UTC(), *source)
				if err != nil {
					runLogf(runID, "sync.ticker source=%s err=%v", *source, err)
					continue
				}
				if runVerboseEnabled() {
					runLogf(runID, "sync.ticker source=%s applied=%d", *source, applied)
				}
			case <-triggerSync:
				applied, err := syncTmuxOnce(runCtx, svc, tmux, time.Now().UTC(), *source)
				if err != nil {
					runLogf(runID, "sync.trigger source=%s err=%v", *source, err)
					continue
				}
				if runVerboseEnabled() {
					runLogf(runID, "sync.trigger source=%s applied=%d", *source, applied)
				}
			}
		}
	}()

	boardRuntime := board.NewService(svc, tmux, os.Getenv("TMUX_PANE"))
	model := tui.NewBoard(boardRuntime, *refresh)
	program := newTeaProgram(model, tea.WithAltScreen())
	_, err = program.Run()
	stop()
	bg.Wait()
	if err != nil {
		runLogf(runID, "ui.exit err=%v", err)
	}
	return err
}

func newRunID() string {
	return strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
}

func runVerboseEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PANEFLEET_OBS_VERBOSE"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func runLogf(runID, format string, args ...any) {
	prefixArgs := []any{runID, time.Now().UTC().Format(time.RFC3339)}
	allArgs := append(prefixArgs, args...)
	fmt.Fprintf(os.Stderr, "panefleet: run_id=%s ts=%s "+format+"\n", allArgs...)
}

func requireTMUXSession() error {
	if strings.TrimSpace(os.Getenv("TMUX")) == "" {
		return errors.New("panefleet must run inside tmux")
	}
	return nil
}

type runtimeAPI struct {
	svc  *panes.Service
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
