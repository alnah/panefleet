package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/alnah/panefleet/internal/state"
	"github.com/alnah/panefleet/internal/store"
)

type Service struct {
	reducer *state.Reducer
	store   store.Store
}

func NewService(reducer *state.Reducer, st store.Store) *Service {
	return &Service{
		reducer: reducer,
		store:   st,
	}
}

func (s *Service) Ingest(ctx context.Context, ev state.Event) (state.PaneState, error) {
	if ev.ID == "" {
		ev.ID = uuid.NewString()
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now().UTC()
	}

	current, ok, err := s.store.GetPaneState(ctx, ev.PaneID)
	if err != nil {
		return state.PaneState{}, err
	}
	if !ok {
		current = state.NewPaneState(ev.PaneID)
		current.LastEventAt = ev.OccurredAt
		current.LastTransitionAt = ev.OccurredAt
	}

	next, err := s.reducer.Apply(current, ev)
	if err != nil {
		return state.PaneState{}, err
	}
	if err := s.store.AppendAndProject(ctx, ev, next); err != nil {
		return state.PaneState{}, err
	}
	return next, nil
}

func (s *Service) StateShow(ctx context.Context, paneID string) (state.PaneState, error) {
	if paneID == "" {
		return state.PaneState{}, errors.New("pane id is required")
	}
	st, ok, err := s.store.GetPaneState(ctx, paneID)
	if err != nil {
		return state.PaneState{}, err
	}
	if !ok {
		return state.PaneState{}, fmt.Errorf("pane not found: %s", paneID)
	}
	return st, nil
}

func (s *Service) StateList(ctx context.Context) ([]state.PaneState, error) {
	return s.store.ListPaneStates(ctx)
}
