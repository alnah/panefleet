package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alnah/panefleet/internal/state"
)

type fakeReader struct {
	list       []state.PaneState
	listErr    error
	setCalls   int
	clearCalls int
	killCalls  int
	respCalls  int
	lastPane   string
}

func (f *fakeReader) StateList(context.Context) ([]state.PaneState, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]state.PaneState, len(f.list))
	copy(out, f.list)
	return out, nil
}

func (f *fakeReader) SetOverride(_ context.Context, paneID string, _ state.Status, _ string) (state.PaneState, error) {
	f.setCalls++
	f.lastPane = paneID
	return state.PaneState{PaneID: paneID, Status: state.StatusStale}, nil
}

func (f *fakeReader) ClearOverride(_ context.Context, paneID, _ string) (state.PaneState, error) {
	f.clearCalls++
	f.lastPane = paneID
	return state.PaneState{PaneID: paneID, Status: state.StatusIdle}, nil
}

func (f *fakeReader) KillPane(_ context.Context, paneID string) error {
	f.killCalls++
	f.lastPane = paneID
	return nil
}

func (f *fakeReader) RespawnPane(_ context.Context, paneID string) error {
	f.respCalls++
	f.lastPane = paneID
	return nil
}

func TestModelFetchSortAndSelection(t *testing.T) {
	reader := &fakeReader{
		list: []state.PaneState{
			{PaneID: "%3", Status: state.StatusIdle},
			{PaneID: "%1", Status: state.StatusRun},
			{PaneID: "%2", Status: state.StatusWait},
		},
	}
	m := New(reader, 100*time.Millisecond, nil)
	msg := m.fetchCmd()()
	updated, _ := m.Update(msg)
	got := updated.(Model)

	if len(got.states) != 3 {
		t.Fatalf("states length = %d, want 3", len(got.states))
	}
	if got.states[0].PaneID != "%2" || got.states[1].PaneID != "%1" || got.states[2].PaneID != "%3" {
		t.Fatalf("unexpected sort order: %#v", got.states)
	}
	if got.lastRefresh.IsZero() {
		t.Fatalf("lastRefresh should be set")
	}
}

func TestModelKeyNavigationAndActions(t *testing.T) {
	reader := &fakeReader{
		list: []state.PaneState{
			{PaneID: "%1", Status: state.StatusRun},
			{PaneID: "%2", Status: state.StatusIdle},
		},
	}
	m := New(reader, 100*time.Millisecond, nil)
	msg := m.fetchCmd()()
	cur, _ := m.Update(msg)
	model := cur.(Model)

	cur, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = cur.(Model)
	if model.selected != 1 {
		t.Fatalf("selected = %d, want 1", model.selected)
	}
	cur, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = cur.(Model)
	if model.selected != 0 {
		t.Fatalf("selected = %d, want 0", model.selected)
	}

	cur, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = cur.(Model)
	action := cmd()
	cur, _ = model.Update(action)
	model = cur.(Model)
	if reader.killCalls != 1 || reader.lastPane != "%1" {
		t.Fatalf("kill not called as expected: calls=%d pane=%s", reader.killCalls, reader.lastPane)
	}

	cur, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = cur.(Model)
	action = cmd()
	cur, _ = model.Update(action)
	model = cur.(Model)
	if reader.respCalls != 1 || reader.lastPane != "%1" {
		t.Fatalf("respawn not called as expected: calls=%d pane=%s", reader.respCalls, reader.lastPane)
	}

	cur, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model = cur.(Model)
	action = cmd()
	cur, _ = model.Update(action)
	model = cur.(Model)
	if reader.setCalls != 1 {
		t.Fatalf("set override calls = %d, want 1", reader.setCalls)
	}

	stale := state.StatusStale
	model.states[0].ManualOverride = &stale
	_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	_ = cmd()
	if reader.clearCalls != 1 {
		t.Fatalf("clear override calls = %d, want 1", reader.clearCalls)
	}
}

func TestModelErrorAndViewAndHelpers(t *testing.T) {
	reader := &fakeReader{listErr: errors.New("boom")}
	m := New(reader, 0, nil)
	if m.interval <= 0 {
		t.Fatalf("default interval should be positive")
	}
	updated, cmd := m.Update(m.fetchCmd()())
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("unexpected command on fetch error branch")
	}
	view := m.View()
	if !strings.Contains(view, "error: boom") {
		t.Fatalf("view should include error, got %q", view)
	}
	if !strings.Contains(view, "(no pane state yet)") {
		t.Fatalf("view should include empty state marker")
	}

	if trim("abcdef", 3) != "abcdef" {
		t.Fatalf("trim should be unchanged for max <= 3")
	}
	if trim("abcdef", 5) != "ab..." {
		t.Fatalf("trim unexpected result")
	}
	if formatRefresh(time.Time{}) != "-" {
		t.Fatalf("formatRefresh zero should be -")
	}
	if got := formatRefresh(time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)); !strings.Contains(got, "2026-03-27T12:00:00Z") {
		t.Fatalf("formatRefresh non-zero mismatch: %q", got)
	}
	if priority(state.StatusWait) >= priority(state.StatusRun) {
		t.Fatalf("priority order should keep WAIT above RUN")
	}
	_ = priority(state.StatusError)
	_ = priority(state.StatusDone)
	_ = priority(state.StatusIdle)
	_ = priority(state.StatusStale)
	if priority(state.Status("UNKNOWN-ELSE")) != 7 {
		t.Fatalf("unexpected default priority")
	}
}

func TestWaitForUpdateCmd(t *testing.T) {
	ch := make(chan state.PaneState, 1)
	ch <- state.PaneState{PaneID: "%1"}
	msg := waitForUpdateCmd(ch)()
	if _, ok := msg.(stateUpdatedMsg); !ok {
		t.Fatalf("expected stateUpdatedMsg, got %T", msg)
	}

	close(ch)
	if got := waitForUpdateCmd(ch)(); got != nil {
		t.Fatalf("expected nil on closed channel, got %T", got)
	}
}

func TestInitReturnsBatch(t *testing.T) {
	reader := &fakeReader{}
	updates := make(chan state.PaneState, 1)
	m := New(reader, 100*time.Millisecond, updates)
	if cmd := m.Init(); cmd == nil {
		t.Fatalf("Init() should return a command")
	}
}

func TestModelUpdateTickAndStateUpdatedAndQuit(t *testing.T) {
	reader := &fakeReader{
		list: []state.PaneState{{PaneID: "%1", Status: state.StatusIdle}},
	}
	updates := make(chan state.PaneState, 1)
	m := New(reader, 10*time.Millisecond, updates)

	updated, cmd := m.Update(tickMsg(time.Now()))
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("tick should schedule batch command")
	}

	updates <- state.PaneState{PaneID: "%1"}
	updated, cmd = m.Update(stateUpdatedMsg{})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("stateUpdated should trigger fetch command")
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("quit key should return quit command")
	}
}

func TestTickCmdReturnsTickMsg(t *testing.T) {
	cmd := tickCmd(time.Millisecond)
	if msg := cmd(); msg == nil {
		t.Fatalf("tickCmd should produce message")
	} else {
		if _, ok := msg.(tickMsg); !ok {
			t.Fatalf("expected tickMsg, got %T", msg)
		}
	}
}

func TestSelectedStateOutOfBounds(t *testing.T) {
	m := New(&fakeReader{}, 10*time.Millisecond, nil)
	if _, ok := m.selectedState(); ok {
		t.Fatalf("selectedState should fail when empty")
	}
	m.states = []state.PaneState{{PaneID: "%1"}}
	m.selected = 10
	if _, ok := m.selectedState(); ok {
		t.Fatalf("selectedState should fail when index out of bounds")
	}
}

func TestViewRendersRowsAndSelection(t *testing.T) {
	m := New(&fakeReader{}, time.Millisecond, nil)
	m.states = []state.PaneState{
		{PaneID: "%1", Status: state.StatusRun, StatusSource: "src", ReasonCode: "reason"},
		{PaneID: "%2", Status: state.StatusDone, StatusSource: "src2", ReasonCode: "reason2"},
	}
	m.selected = 1
	view := m.View()
	if !strings.Contains(view, "%1") || !strings.Contains(view, "%2") {
		t.Fatalf("view rows missing panes: %q", view)
	}
	if !strings.Contains(view, ">   %2") {
		t.Fatalf("view selection marker missing: %q", view)
	}
}
