package panes

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alnah/panefleet/internal/state"
)

type failingStore struct {
	getErr  error
	listErr error
	putErr  error
}

func (f failingStore) Init(context.Context) error { return nil }
func (f failingStore) Close() error               { return nil }
func (f failingStore) GetPaneState(_ context.Context, paneID string) (state.PaneState, bool, error) {
	if f.getErr != nil {
		return state.PaneState{}, false, f.getErr
	}
	return state.NewPaneState(paneID), true, nil
}
func (f failingStore) ListPaneStates(context.Context) ([]state.PaneState, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return nil, nil
}
func (f failingStore) AppendAndProject(context.Context, state.Event, state.PaneState) error {
	return f.putErr
}

func TestServiceErrorScopes(t *testing.T) {
	reducer, err := state.NewReducer(state.Config{
		DoneRecentWindow: 10 * time.Minute,
		StaleWindow:      45 * time.Minute,
	})
	if err != nil {
		t.Fatalf("new reducer: %v", err)
	}

	svcLookup := NewService(reducer, failingStore{getErr: errors.New("db lookup fail")})
	_, err = svcLookup.Ingest(context.Background(), state.Event{
		ID:         "evt-1",
		PaneID:     "%42",
		Kind:       state.EventPaneStarted,
		OccurredAt: time.Now().UTC(),
	})
	if err == nil || !strings.Contains(err.Error(), "ingest lookup (pane=%42 kind=PaneStarted event_id=evt-1)") {
		t.Fatalf("expected scoped lookup error, got: %v", err)
	}

	svcPersist := NewService(reducer, failingStore{putErr: errors.New("db write fail")})
	_, err = svcPersist.Ingest(context.Background(), state.Event{
		ID:         "evt-2",
		PaneID:     "%7",
		Kind:       state.EventPaneStarted,
		OccurredAt: time.Now().UTC().Add(1 * time.Second),
	})
	if err == nil || !strings.Contains(err.Error(), "ingest persist (pane=%7 kind=PaneStarted event_id=evt-2)") {
		t.Fatalf("expected scoped persist error, got: %v", err)
	}

	svcList := NewService(reducer, failingStore{listErr: errors.New("db list fail")})
	_, err = svcList.StateList(context.Background())
	if err == nil || !strings.Contains(err.Error(), "state-list: db list fail") {
		t.Fatalf("expected state-list wrapped error, got: %v", err)
	}
}
