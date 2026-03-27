package store

import (
	"context"

	"github.com/alnah/panefleet/internal/state"
)

type Store interface {
	Init(ctx context.Context) error
	AppendAndProject(ctx context.Context, ev state.Event, st state.PaneState) error
	GetPaneState(ctx context.Context, paneID string) (state.PaneState, bool, error)
	ListPaneStates(ctx context.Context) ([]state.PaneState, error)
	Close() error
}
