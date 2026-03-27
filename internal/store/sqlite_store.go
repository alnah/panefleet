package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alnah/panefleet/internal/state"
	_ "modernc.org/sqlite"
)

// SQLiteStore is the default local Store implementation for Panefleet.
type SQLiteStore struct {
	db  *sql.DB
	dsn string
}

// NewSQLiteStore creates a single-writer sqlite handle tuned for local,
// ordered event streams.
func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(0)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return &SQLiteStore{db: db, dsn: dsn}, nil
}

// Close releases the underlying DB handle.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Init ensures the event log and projection schema exist before writes.
func (s *SQLiteStore) Init(ctx context.Context) error {
	if err := s.configureConnection(ctx); err != nil {
		return err
	}
	const ddl = `
CREATE TABLE IF NOT EXISTS events (
  event_id TEXT PRIMARY KEY,
  pane_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  occurred_at TEXT NOT NULL,
  source TEXT,
  reason_code TEXT,
  payload_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS pane_state (
  pane_id TEXT PRIMARY KEY,
  status TEXT NOT NULL,
  status_source TEXT NOT NULL,
  reason_code TEXT NOT NULL,
  version INTEGER NOT NULL,
  last_event_at TEXT NOT NULL,
  last_transition_at TEXT NOT NULL,
  last_exit_code INTEGER NULL,
  manual_override TEXT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_pane_time ON events (pane_id, occurred_at);
`
	_, err := s.db.ExecContext(ctx, ddl)
	return err
}

func (s *SQLiteStore) configureConnection(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA synchronous = NORMAL`); err != nil {
		return err
	}
	var mode string
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode = WAL`).Scan(&mode); err != nil {
		return err
	}
	if expectsWAL(s.dsn) && !strings.EqualFold(mode, "wal") {
		return fmt.Errorf("sqlite journal_mode=%q, want wal", mode)
	}
	return nil
}

// AppendAndProject persists one event and its resulting projection in a single
// transaction to keep idempotence and projection consistency.
func (s *SQLiteStore) AppendAndProject(ctx context.Context, ev state.Event, st state.PaneState) error {
	if err := ev.Validate(); err != nil {
		return err
	}

	payload, err := json.Marshal(map[string]any{
		"exit_code":   ev.ExitCode,
		"override_to": ev.OverrideTo,
	})
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO events(event_id, pane_id, kind, occurred_at, source, reason_code, payload_json)
VALUES(?, ?, ?, ?, ?, ?, ?)`,
		ev.ID, ev.PaneID, string(ev.Kind), formatTime(ev.OccurredAt), ev.Source, ev.ReasonCode, string(payload),
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	// Duplicate event_id: idempotent no-op.
	if affected == 0 {
		return tx.Commit()
	}
	if err := validateProjection(ev, st); err != nil {
		return err
	}

	var manualOverride *string
	if st.ManualOverride != nil {
		v := string(*st.ManualOverride)
		manualOverride = &v
	}

	_, err = tx.ExecContext(ctx, `
	INSERT INTO pane_state(
	  pane_id, status, status_source, reason_code, version, last_event_at, last_transition_at, last_exit_code, manual_override
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(pane_id) DO UPDATE SET
  status=excluded.status,
  status_source=excluded.status_source,
  reason_code=excluded.reason_code,
  version=excluded.version,
	  last_event_at=excluded.last_event_at,
	  last_transition_at=excluded.last_transition_at,
	  last_exit_code=excluded.last_exit_code,
	  manual_override=excluded.manual_override
	WHERE pane_state.version = excluded.version - 1`,
		st.PaneID,
		string(st.Status),
		st.StatusSource,
		st.ReasonCode,
		st.Version,
		formatTime(st.LastEventAt),
		formatTime(st.LastTransitionAt),
		st.LastExitCode,
		manualOverride,
	)
	if err != nil {
		return err
	}
	affected, err = rowsAffectedOne(tx, "SELECT changes()")
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrConcurrentWrite
	}

	return tx.Commit()
}

// GetPaneState returns one pane projection by id, plus a found flag.
func (s *SQLiteStore) GetPaneState(ctx context.Context, paneID string) (state.PaneState, bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT pane_id, status, status_source, reason_code, version, last_event_at, last_transition_at, last_exit_code, manual_override
FROM pane_state WHERE pane_id = ?`, paneID)

	st, ok, err := scanPaneState(row)
	return st, ok, err
}

// ListPaneStates returns all pane projections in deterministic pane-id order.
func (s *SQLiteStore) ListPaneStates(ctx context.Context) ([]state.PaneState, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT pane_id, status, status_source, reason_code, version, last_event_at, last_transition_at, last_exit_code, manual_override
FROM pane_state ORDER BY pane_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]state.PaneState, 0)
	for rows.Next() {
		st, err := scanPaneStateFromRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPaneState(row rowScanner) (state.PaneState, bool, error) {
	st, err := scanPaneStateInternal(row)
	if err == sql.ErrNoRows {
		return state.PaneState{}, false, nil
	}
	if err != nil {
		return state.PaneState{}, false, err
	}
	return st, true, nil
}

func scanPaneStateFromRows(rows *sql.Rows) (state.PaneState, error) {
	return scanPaneStateInternal(rows)
}

func scanPaneStateInternal(row rowScanner) (state.PaneState, error) {
	var (
		paneID         string
		rawStatus      string
		statusSource   string
		reasonCode     string
		version        uint64
		lastEventAtRaw string
		lastTransAtRaw string
		lastExitCode   sql.NullInt64
		manualOverride sql.NullString
	)
	if err := row.Scan(
		&paneID,
		&rawStatus,
		&statusSource,
		&reasonCode,
		&version,
		&lastEventAtRaw,
		&lastTransAtRaw,
		&lastExitCode,
		&manualOverride,
	); err != nil {
		return state.PaneState{}, err
	}

	parsedStatus, err := state.ParseStatus(rawStatus)
	if err != nil {
		return state.PaneState{}, err
	}
	lastEventAt, err := parseTime(lastEventAtRaw)
	if err != nil {
		return state.PaneState{}, err
	}
	lastTransitionAt, err := parseTime(lastTransAtRaw)
	if err != nil {
		return state.PaneState{}, err
	}

	var exitCodePtr *int
	if lastExitCode.Valid {
		v := int(lastExitCode.Int64)
		exitCodePtr = &v
	}

	var manualOverridePtr *state.Status
	if manualOverride.Valid {
		o, err := state.ParseStatus(manualOverride.String)
		if err != nil {
			return state.PaneState{}, err
		}
		manualOverridePtr = &o
	}

	return state.PaneState{
		PaneID:           paneID,
		Status:           parsedStatus,
		StatusSource:     statusSource,
		ReasonCode:       reasonCode,
		Version:          version,
		LastEventAt:      lastEventAt,
		LastTransitionAt: lastTransitionAt,
		LastExitCode:     exitCodePtr,
		ManualOverride:   manualOverridePtr,
	}, nil
}

func formatTime(ts time.Time) string {
	return ts.UTC().Format(time.RFC3339Nano)
}

func parseTime(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, raw)
}

func rowsAffectedOne(tx *sql.Tx, query string) (int64, error) {
	var changes int64
	if err := tx.QueryRow(query).Scan(&changes); err != nil {
		return 0, err
	}
	return changes, nil
}

func expectsWAL(dsn string) bool {
	lower := strings.ToLower(strings.TrimSpace(dsn))
	if lower == "" || strings.Contains(lower, ":memory:") {
		return false
	}
	return !strings.Contains(lower, "mode=memory")
}

func validateProjection(ev state.Event, st state.PaneState) error {
	if st.PaneID == "" {
		return fmt.Errorf("projection pane_id is required")
	}
	if st.PaneID != ev.PaneID {
		return fmt.Errorf("projection pane mismatch: event=%s projection=%s", ev.PaneID, st.PaneID)
	}
	if !st.Status.Valid() {
		return fmt.Errorf("invalid projection status: %s", st.Status)
	}
	if st.ManualOverride != nil && !st.ManualOverride.Valid() {
		return fmt.Errorf("invalid projection manual override: %s", *st.ManualOverride)
	}
	if st.Version == 0 {
		return fmt.Errorf("projection version must be > 0")
	}
	if st.LastEventAt.IsZero() {
		return fmt.Errorf("projection last_event_at is required")
	}
	if st.LastTransitionAt.IsZero() {
		return fmt.Errorf("projection last_transition_at is required")
	}
	if !st.LastEventAt.Equal(ev.OccurredAt) {
		return fmt.Errorf("projection last_event_at %s does not match event occurred_at %s", formatTime(st.LastEventAt), formatTime(ev.OccurredAt))
	}
	if st.LastTransitionAt.After(st.LastEventAt) {
		return fmt.Errorf("projection last_transition_at %s cannot be after last_event_at %s", formatTime(st.LastTransitionAt), formatTime(st.LastEventAt))
	}
	return nil
}
