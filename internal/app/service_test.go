package app

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alnah/panefleet/internal/state"
	"github.com/alnah/panefleet/internal/store"
)

func newService(t *testing.T) *Service {
	t.Helper()
	st, err := store.NewSQLiteStore("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("init store: %v", err)
	}

	reducer, err := state.NewReducer(state.Config{
		DoneRecentWindow: 10 * time.Minute,
		StaleWindow:      45 * time.Minute,
	})
	if err != nil {
		t.Fatalf("new reducer: %v", err)
	}
	return NewService(reducer, st)
}

func TestServiceIngestAndStateShow(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 26, 18, 0, 0, 0, time.UTC)

	_, err := svc.Ingest(ctx, state.Event{
		PaneID:     "%11",
		Kind:       state.EventPaneStarted,
		OccurredAt: at,
		Source:     "adapter:test",
	})
	if err != nil {
		t.Fatalf("ingest started: %v", err)
	}

	got, err := svc.StateShow(ctx, "%11")
	if err != nil {
		t.Fatalf("state-show: %v", err)
	}
	if got.Status != state.StatusRun {
		t.Fatalf("want RUN, got %s", got.Status)
	}
}

func TestServiceStateList(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 26, 18, 30, 0, 0, time.UTC)

	events := []state.Event{
		{
			PaneID:     "%21",
			Kind:       state.EventPaneStarted,
			OccurredAt: base,
		},
		{
			PaneID:     "%22",
			Kind:       state.EventPaneWaiting,
			OccurredAt: base.Add(1 * time.Second),
		},
	}
	for _, ev := range events {
		if _, err := svc.Ingest(ctx, ev); err != nil {
			t.Fatalf("ingest event: %v", err)
		}
	}

	all, err := svc.StateList(ctx)
	if err != nil {
		t.Fatalf("state-list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 states, got %d", len(all))
	}
}

func TestServiceSubscribePublishesOnIngest(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	ch, cancel := svc.Subscribe()
	defer cancel()

	at := time.Date(2026, 3, 26, 19, 0, 0, 0, time.UTC)
	if _, err := svc.Ingest(ctx, state.Event{
		PaneID:     "%31",
		Kind:       state.EventPaneStarted,
		OccurredAt: at,
	}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	select {
	case got := <-ch:
		if got.PaneID != "%31" {
			t.Fatalf("unexpected pane id: %s", got.PaneID)
		}
		if got.Status != state.StatusRun {
			t.Fatalf("unexpected status: %s", got.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for publish")
	}
}

type concurrencyStore struct {
	mu          sync.Mutex
	state       state.PaneState
	hasState    bool
	inFlight    int32
	maxInFlight int32
}

func (s *concurrencyStore) Init(context.Context) error { return nil }
func (s *concurrencyStore) Close() error               { return nil }

func (s *concurrencyStore) GetPaneState(_ context.Context, paneID string) (state.PaneState, bool, error) {
	s.enter()
	defer s.leave()

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasState {
		return state.PaneState{}, false, nil
	}
	return s.state, true, nil
}

func (s *concurrencyStore) AppendAndProject(_ context.Context, _ state.Event, st state.PaneState) error {
	s.enter()
	defer s.leave()

	s.mu.Lock()
	s.state = st
	s.hasState = true
	s.mu.Unlock()
	return nil
}

func (s *concurrencyStore) ListPaneStates(context.Context) ([]state.PaneState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasState {
		return nil, nil
	}
	return []state.PaneState{s.state}, nil
}

func (s *concurrencyStore) enter() {
	current := atomic.AddInt32(&s.inFlight, 1)
	for {
		maxSeen := atomic.LoadInt32(&s.maxInFlight)
		if current <= maxSeen || atomic.CompareAndSwapInt32(&s.maxInFlight, maxSeen, current) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
}

func (s *concurrencyStore) leave() {
	atomic.AddInt32(&s.inFlight, -1)
}

func TestServiceIngestSerializesByPane(t *testing.T) {
	reducer, err := state.NewReducer(state.Config{
		DoneRecentWindow: 10 * time.Minute,
		StaleWindow:      45 * time.Minute,
	})
	if err != nil {
		t.Fatalf("new reducer: %v", err)
	}
	st := &concurrencyStore{}
	svc := NewService(reducer, st)

	base := time.Now().UTC()
	events := []state.Event{
		{
			ID:         "evt-serial-1",
			PaneID:     "%91",
			Kind:       state.EventPaneStarted,
			OccurredAt: base,
		},
		{
			ID:         "evt-serial-2",
			PaneID:     "%91",
			Kind:       state.EventPaneWaiting,
			OccurredAt: base.Add(time.Second),
		},
	}

	errCh := make(chan error, len(events))
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := svc.Ingest(context.Background(), events[0])
		errCh <- err
	}()
	time.Sleep(5 * time.Millisecond)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := svc.Ingest(context.Background(), events[1])
		errCh <- err
	}()
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("ingest: %v", err)
		}
	}
	if got := atomic.LoadInt32(&st.maxInFlight); got != 1 {
		t.Fatalf("service should serialize store access per pane, max concurrent calls=%d", got)
	}

	got, err := svc.StateShow(context.Background(), "%91")
	if err != nil {
		t.Fatalf("StateShow: %v", err)
	}
	if got.Version != 2 {
		t.Fatalf("want version 2 after sequential projection, got %d", got.Version)
	}
	if got.Status != state.StatusWait {
		t.Fatalf("want WAIT after second event, got %s", got.Status)
	}
}

func TestServiceDuplicateEventReturnsPersistedState(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	ch, cancel := svc.Subscribe()
	defer cancel()

	base := time.Now().UTC()
	first, err := svc.Ingest(ctx, state.Event{
		ID:         "dup-event-1",
		PaneID:     "%77",
		Kind:       state.EventPaneStarted,
		OccurredAt: base,
		Source:     "adapter:test",
	})
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	if first.Version != 1 || first.Status != state.StatusRun {
		t.Fatalf("unexpected first state: %+v", first)
	}
	<-ch

	second, err := svc.Ingest(ctx, state.Event{
		ID:         "dup-event-1",
		PaneID:     "%77",
		Kind:       state.EventPaneWaiting,
		OccurredAt: base.Add(time.Second),
		Source:     "adapter:test",
	})
	if err != nil {
		t.Fatalf("duplicate ingest: %v", err)
	}
	if second.Version != 1 {
		t.Fatalf("duplicate ingest should return persisted version 1, got %d", second.Version)
	}
	if second.Status != state.StatusRun {
		t.Fatalf("duplicate ingest should return persisted RUN, got %s", second.Status)
	}

	select {
	case got := <-ch:
		if got.Version != 1 || got.Status != state.StatusRun {
			t.Fatalf("published ghost state: %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for duplicate publish")
	}
}

type retryStore struct {
	mu          sync.Mutex
	state       state.PaneState
	conflicted  bool
	appendCalls int
}

func (s *retryStore) Init(context.Context) error { return nil }
func (s *retryStore) Close() error               { return nil }

func (s *retryStore) GetPaneState(context.Context, string) (state.PaneState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, true, nil
}

func (s *retryStore) AppendAndProject(_ context.Context, _ state.Event, st state.PaneState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendCalls++
	if !s.conflicted {
		s.conflicted = true
		s.state = state.PaneState{
			PaneID:           st.PaneID,
			Status:           state.StatusWait,
			StatusSource:     "adapter:other-writer",
			ReasonCode:       "pane.waiting",
			Version:          2,
			LastEventAt:      st.LastEventAt.Add(-1 * time.Second),
			LastTransitionAt: st.LastEventAt.Add(-1 * time.Second),
		}
		return store.ErrConcurrentWrite
	}
	s.state = st
	return nil
}

func (s *retryStore) ListPaneStates(context.Context) ([]state.PaneState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return []state.PaneState{s.state}, nil
}

func TestServiceRetriesAfterConcurrentWrite(t *testing.T) {
	reducer, err := state.NewReducer(state.Config{
		DoneRecentWindow: 10 * time.Minute,
		StaleWindow:      45 * time.Minute,
	})
	if err != nil {
		t.Fatalf("new reducer: %v", err)
	}

	base := time.Now().UTC()
	st := &retryStore{
		state: state.PaneState{
			PaneID:           "%88",
			Status:           state.StatusRun,
			StatusSource:     "adapter:test",
			ReasonCode:       "pane.started",
			Version:          1,
			LastEventAt:      base,
			LastTransitionAt: base,
		},
	}
	svc := NewService(reducer, st)

	exitCode := 3
	got, err := svc.Ingest(context.Background(), state.Event{
		ID:         "retry-1",
		PaneID:     "%88",
		Kind:       state.EventPaneExited,
		OccurredAt: base.Add(2 * time.Second),
		ExitCode:   &exitCode,
		Source:     "adapter:test",
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if st.appendCalls != 2 {
		t.Fatalf("expected one retry, appendCalls=%d", st.appendCalls)
	}
	if got.Version != 3 {
		t.Fatalf("expected version 3 after retry, got %d", got.Version)
	}
	if got.Status != state.StatusError {
		t.Fatalf("expected ERROR after retry, got %s", got.Status)
	}
}
