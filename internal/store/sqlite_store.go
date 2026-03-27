package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alnah/panefleet/internal/state"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(0)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) Init(ctx context.Context) error {
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

func (s *SQLiteStore) AppendAndProject(ctx context.Context, ev state.Event, st state.PaneState) error {
	if err := ev.Validate(); err != nil {
		return err
	}
	if !st.Status.Valid() {
		return fmt.Errorf("invalid projection status: %s", st.Status)
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
  manual_override=excluded.manual_override`,
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

	return tx.Commit()
}

func (s *SQLiteStore) GetPaneState(ctx context.Context, paneID string) (state.PaneState, bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT pane_id, status, status_source, reason_code, version, last_event_at, last_transition_at, last_exit_code, manual_override
FROM pane_state WHERE pane_id = ?`, paneID)

	st, ok, err := scanPaneState(row)
	return st, ok, err
}

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
