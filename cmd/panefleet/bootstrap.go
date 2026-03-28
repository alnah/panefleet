package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alnah/panefleet/internal/panes"
	"github.com/alnah/panefleet/internal/state"
	"github.com/alnah/panefleet/internal/store"
)

func newService(ctx context.Context) (*panes.Service, func(), error) {
	dbPath, err := resolveDBPath()
	if err != nil {
		return nil, nil, err
	}
	if shouldManageDBPath(dbPath) {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
			return nil, nil, err
		}
	}

	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		return nil, nil, err
	}
	if err := st.Init(ctx); err != nil {
		_ = st.Close()
		return nil, nil, err
	}
	if shouldManageDBPath(dbPath) {
		if err := os.Chmod(dbPath, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = st.Close()
			return nil, nil, fmt.Errorf("secure db file permissions: %w", err)
		}
	}
	reducer, err := state.NewReducer(state.Config{
		DoneRecentWindow: 10 * time.Minute,
		StaleWindow:      45 * time.Minute,
	})
	if err != nil {
		_ = st.Close()
		return nil, nil, err
	}

	return panes.NewService(reducer, st), func() { _ = st.Close() }, nil
}

func resolveDBPath() (string, error) {
	if dbPath := strings.TrimSpace(os.Getenv("PANEFLEET_DB_PATH")); dbPath != "" {
		return dbPath, nil
	}
	stateHome := strings.TrimSpace(os.Getenv("XDG_STATE_HOME"))
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		stateHome = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateHome, "panefleet", "panefleet.db"), nil
}

func shouldManageDBPath(dbPath string) bool {
	if dbPath == "" || dbPath == ":memory:" {
		return false
	}
	if strings.HasPrefix(dbPath, "file:") {
		return false
	}
	if strings.Contains(dbPath, "?") {
		return false
	}
	return true
}
