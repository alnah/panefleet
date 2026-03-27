package store

import (
	"context"
	"errors"

	"github.com/alnah/panefleet/internal/state"
)

// ErrConcurrentWrite reports that another writer advanced pane_state between
// the caller's read and write attempt.
var ErrConcurrentWrite = errors.New("concurrent pane_state write")

// Store defines the persistence boundary for append+project state handling.
type Store interface {
	Init(ctx context.Context) error
	AppendAndProject(ctx context.Context, ev state.Event, st state.PaneState) error
	GetPaneState(ctx context.Context, paneID string) (state.PaneState, bool, error)
	ListPaneStates(ctx context.Context) ([]state.PaneState, error)
	Close() error
}
