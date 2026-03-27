package board

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/alnah/panefleet/internal/tmuxctl"
)

func TestRowsFallbackToLatestClaudeProjectTranscriptMetrics(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := "/Users/alexis/workspace/fle"
	transcriptDir := filepath.Join(homeDir, ".claude", "projects", "-Users-alexis-workspace-fle")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatalf("mkdir claude transcript dir: %v", err)
	}
	transcriptPath := filepath.Join(transcriptDir, "session.jsonl")
	rawTranscript := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"hello"}}`,
		`{"type":"assistant","message":{"role":"assistant","model":"claude-sonnet-4-6","usage":{"input_tokens":3,"cache_creation_input_tokens":18896,"cache_read_input_tokens":0,"output_tokens":659}}}`,
	}, "\n")
	if err := os.WriteFile(transcriptPath, []byte(rawTranscript), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	t.Setenv("HOME", homeDir)

	svc := NewService(
		&fakeStateSource{},
		&fakeTMUX{
			snapshot: []tmuxctl.BoardPane{
				{
					PaneID:      "%1",
					SessionName: "work",
					WindowIndex: "4",
					WindowName:  "claude",
					PaneIndex:   "0",
					Command:     "2.1.85",
					Title:       "claude",
					Path:        projectPath,
				},
			},
		},
		"",
	)

	rows, err := svc.Rows(context.Background())
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].TokensUsed == nil || *rows[0].TokensUsed != 19558 {
		t.Fatalf("rows[0].TokensUsed = %v, want 19558", rows[0].TokensUsed)
	}
	if rows[0].ContextLeftPct == nil || *rows[0].ContextLeftPct != 90 {
		t.Fatalf("rows[0].ContextLeftPct = %v, want 90", rows[0].ContextLeftPct)
	}
}

func TestRowsFallbackToLatestOpenCodeSessionMetrics(t *testing.T) {
	homeDir := t.TempDir()
	projectPath := "/Users/alexis/workspace/fle"
	t.Setenv("HOME", homeDir)

	codexHome := t.TempDir()
	writeBoardModelsCache(t, codexHome, "gpt-5.4", 272000, 95)
	t.Setenv("CODEX_HOME", codexHome)

	writeOpenCodeFixture(t, homeDir, projectPath)

	svc := NewService(
		&fakeStateSource{},
		&fakeTMUX{
			snapshot: []tmuxctl.BoardPane{
				{
					PaneID:      "%2",
					SessionName: "work",
					WindowIndex: "5",
					WindowName:  "open",
					PaneIndex:   "0",
					Command:     "opencode",
					Title:       "open",
					Path:        projectPath,
				},
			},
		},
		"",
	)

	rows, err := svc.Rows(context.Background())
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].TokensUsed == nil || *rows[0].TokensUsed != 15608 {
		t.Fatalf("rows[0].TokensUsed = %v, want 15608", rows[0].TokensUsed)
	}
	if rows[0].ContextLeftPct == nil || *rows[0].ContextLeftPct != 94 {
		t.Fatalf("rows[0].ContextLeftPct = %v, want 94", rows[0].ContextLeftPct)
	}
}

func writeBoardModelsCache(t *testing.T, codexHome, model string, contextWindow, effectivePct int) {
	t.Helper()
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	modelsCache := codexModelsCache{
		Models: []codexModelInfo{
			{
				Slug:                          model,
				ContextWindow:                 contextWindow,
				EffectiveContextWindowPercent: effectivePct,
			},
		},
	}
	raw, err := json.Marshal(modelsCache)
	if err != nil {
		t.Fatalf("marshal models cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "models_cache.json"), raw, 0o600); err != nil {
		t.Fatalf("write models cache: %v", err)
	}
}

func writeOpenCodeFixture(t *testing.T, homeDir, projectPath string) {
	t.Helper()

	dbPath := filepath.Join(homeDir, ".local", "share", "opencode", "opencode.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir opencode db dir: %v", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open opencode db: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE session (id TEXT PRIMARY KEY, directory TEXT NOT NULL, time_updated INTEGER NOT NULL);`,
		`CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);`,
		`CREATE TABLE part (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec schema %q: %v", stmt, err)
		}
	}

	assistantData := `{"role":"assistant","modelID":"gpt-5.4","tokens":{"total":15608,"input":15482,"output":126}}`
	if _, err := db.Exec(`INSERT INTO session (id, directory, time_updated) VALUES (?, ?, ?)`, "ses_1", projectPath, 1774650842218); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO message (id, session_id, time_updated, data) VALUES (?, ?, ?, ?)`, "msg_1", "ses_1", 1774650842165, assistantData); err != nil {
		t.Fatalf("insert message: %v", err)
	}
}
