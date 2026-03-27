package board

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/alnah/panefleet/internal/tmuxctl"
)

type claudeMetricsResolver struct {
	findLatestTranscript   func(string) (string, bool, error)
	lookupTranscriptMetric func(string) (codexMetrics, bool, error)
}

func newClaudeMetricsResolver() *claudeMetricsResolver {
	return &claudeMetricsResolver{
		findLatestTranscript:   findLatestClaudeTranscript,
		lookupTranscriptMetric: lookupClaudeTranscriptMetrics,
	}
}

func (r *claudeMetricsResolver) resolve(_ context.Context, pane tmuxctl.BoardPane) (codexMetrics, bool, error) {
	projectPath := strings.TrimSpace(pane.Path)
	if projectPath == "" {
		return codexMetrics{}, false, nil
	}
	transcriptPath, ok, err := r.findLatestTranscript(projectPath)
	if err != nil || !ok {
		return codexMetrics{}, ok, err
	}
	return r.lookupTranscriptMetric(transcriptPath)
}

func findLatestClaudeTranscript(projectPath string) (string, bool, error) {
	projectSlug := claudeProjectSlug(projectPath)
	if projectSlug == "" {
		return "", false, nil
	}

	dir := filepath.Join(claudeProjectsDir(), projectSlug)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read claude project dir %s: %w", dir, err)
	}

	type candidate struct {
		path    string
		modTime int64
	}
	candidates := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{
			path:    filepath.Join(dir, entry.Name()),
			modTime: info.ModTime().UnixNano(),
		})
	}
	if len(candidates) == 0 {
		return "", false, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime == candidates[j].modTime {
			return candidates[i].path > candidates[j].path
		}
		return candidates[i].modTime > candidates[j].modTime
	})
	return candidates[0].path, true, nil
}

func lookupClaudeTranscriptMetrics(transcriptPath string) (codexMetrics, bool, error) {
	raw, err := os.ReadFile(transcriptPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return codexMetrics{}, false, nil
		}
		return codexMetrics{}, false, fmt.Errorf("read claude transcript %s: %w", transcriptPath, err)
	}

	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		message := claudeMapValue(entry["message"])
		if claudeStringValue(message["role"]) != "assistant" {
			continue
		}
		tokensUsed, ok := claudeUsageTotal(claudeMapValue(message["usage"]))
		if !ok {
			continue
		}
		metrics := codexMetrics{TokensUsed: codexIntPtr(tokensUsed)}
		if contextWindow, ok := providerContextWindow(claudeStringValue(message["model"])); ok {
			metrics.ContextLeftPct = codexIntPtr(remainingContextPercent(tokensUsed, contextWindow))
		}
		return metrics, true, nil
	}

	return codexMetrics{}, false, nil
}

func claudeProjectSlug(projectPath string) string {
	projectPath = filepath.Clean(strings.TrimSpace(projectPath))
	if projectPath == "" || projectPath == "." {
		return ""
	}
	return strings.ReplaceAll(projectPath, string(os.PathSeparator), "-")
}

func claudeProjectsDir() string {
	return filepath.Join(userHomeDir(), ".claude", "projects")
}

func claudeUsageTotal(usage map[string]any) (int, bool) {
	inputTokens, hasInput := claudeIntValue(usage["input_tokens"])
	outputTokens, hasOutput := claudeIntValue(usage["output_tokens"])
	cacheCreate, _ := claudeIntValue(usage["cache_creation_input_tokens"])
	cacheRead, _ := claudeIntValue(usage["cache_read_input_tokens"])
	if !hasInput && !hasOutput {
		return 0, false
	}
	return inputTokens + outputTokens + cacheCreate + cacheRead, true
}

type openCodeMetricsResolver struct {
	lookupSessionMetrics func(string) (codexMetrics, bool, error)
}

func newOpenCodeMetricsResolver() *openCodeMetricsResolver {
	return &openCodeMetricsResolver{
		lookupSessionMetrics: lookupOpenCodeSessionMetrics,
	}
}

func (r *openCodeMetricsResolver) resolve(_ context.Context, pane tmuxctl.BoardPane) (codexMetrics, bool, error) {
	projectPath := strings.TrimSpace(pane.Path)
	if projectPath == "" {
		return codexMetrics{}, false, nil
	}
	return r.lookupSessionMetrics(projectPath)
}

func lookupOpenCodeSessionMetrics(projectPath string) (codexMetrics, bool, error) {
	db, err := sql.Open("sqlite", sqliteReadOnlyDSN(opencodeDBPath()))
	if err != nil {
		return codexMetrics{}, false, fmt.Errorf("open opencode db: %w", err)
	}
	defer db.Close()

	sessionID, err := latestOpenCodeSessionID(db, projectPath)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return codexMetrics{}, false, nil
		}
		return codexMetrics{}, false, err
	}

	messageRecord, hasMessage, err := latestOpenCodeMessageMetrics(db, sessionID)
	if err != nil {
		return codexMetrics{}, false, err
	}
	partRecord, hasPart, err := latestOpenCodePartMetrics(db, sessionID)
	if err != nil {
		return codexMetrics{}, false, err
	}

	switch {
	case hasMessage && hasPart:
		if partRecord.TimeUpdated > messageRecord.TimeUpdated && partRecord.Metrics.TokensUsed != nil {
			if partRecord.Metrics.ContextLeftPct == nil {
				partRecord.Metrics.ContextLeftPct = messageRecord.Metrics.ContextLeftPct
			}
			return partRecord.Metrics, true, nil
		}
		return messageRecord.Metrics, true, nil
	case hasMessage:
		return messageRecord.Metrics, true, nil
	case hasPart:
		return partRecord.Metrics, true, nil
	default:
		return codexMetrics{}, false, nil
	}
}

type openCodeMetricRecord struct {
	Metrics     codexMetrics
	TimeUpdated int64
}

func latestOpenCodeSessionID(db *sql.DB, projectPath string) (string, error) {
	row := db.QueryRow(`
SELECT id
FROM session
WHERE directory = ? OR ? LIKE directory || '/%'
ORDER BY LENGTH(directory) DESC, time_updated DESC
LIMIT 1
`, projectPath, projectPath)

	var sessionID string
	if err := row.Scan(&sessionID); err != nil {
		return "", fmt.Errorf("read latest opencode session for %s: %w", projectPath, err)
	}
	return sessionID, nil
}

func latestOpenCodeMessageMetrics(db *sql.DB, sessionID string) (openCodeMetricRecord, bool, error) {
	rows, err := db.Query(`
SELECT data, time_updated
FROM message
WHERE session_id = ?
ORDER BY time_updated DESC
LIMIT 20
`, sessionID)
	if err != nil {
		return openCodeMetricRecord{}, false, fmt.Errorf("query opencode messages for %s: %w", sessionID, err)
	}
	defer rows.Close()

	for rows.Next() {
		var raw string
		var updated int64
		if err := rows.Scan(&raw, &updated); err != nil {
			return openCodeMetricRecord{}, false, fmt.Errorf("scan opencode message metrics for %s: %w", sessionID, err)
		}
		record, ok := decodeOpenCodeMessageMetrics(raw, updated)
		if ok {
			return record, true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return openCodeMetricRecord{}, false, fmt.Errorf("iterate opencode messages for %s: %w", sessionID, err)
	}
	return openCodeMetricRecord{}, false, nil
}

func latestOpenCodePartMetrics(db *sql.DB, sessionID string) (openCodeMetricRecord, bool, error) {
	rows, err := db.Query(`
SELECT data, time_updated
FROM part
WHERE session_id = ?
ORDER BY time_updated DESC
LIMIT 20
`, sessionID)
	if err != nil {
		return openCodeMetricRecord{}, false, fmt.Errorf("query opencode parts for %s: %w", sessionID, err)
	}
	defer rows.Close()

	for rows.Next() {
		var raw string
		var updated int64
		if err := rows.Scan(&raw, &updated); err != nil {
			return openCodeMetricRecord{}, false, fmt.Errorf("scan opencode part metrics for %s: %w", sessionID, err)
		}
		record, ok := decodeOpenCodePartMetrics(raw, updated)
		if ok {
			return record, true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return openCodeMetricRecord{}, false, fmt.Errorf("iterate opencode parts for %s: %w", sessionID, err)
	}
	return openCodeMetricRecord{}, false, nil
}

func decodeOpenCodeMessageMetrics(raw string, updated int64) (openCodeMetricRecord, bool) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return openCodeMetricRecord{}, false
	}
	if claudeStringValue(payload["role"]) != "assistant" {
		return openCodeMetricRecord{}, false
	}
	tokensUsed, ok := claudeIntValue(claudeMapValue(payload["tokens"])["total"])
	if !ok {
		return openCodeMetricRecord{}, false
	}

	record := openCodeMetricRecord{
		Metrics:     codexMetrics{TokensUsed: codexIntPtr(tokensUsed)},
		TimeUpdated: updated,
	}
	if contextWindow, ok := providerContextWindow(claudeStringValue(payload["modelID"])); ok {
		record.Metrics.ContextLeftPct = codexIntPtr(remainingContextPercent(tokensUsed, contextWindow))
	}
	return record, true
}

func decodeOpenCodePartMetrics(raw string, updated int64) (openCodeMetricRecord, bool) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return openCodeMetricRecord{}, false
	}
	tokensUsed, ok := claudeIntValue(claudeMapValue(payload["tokens"])["total"])
	if !ok {
		return openCodeMetricRecord{}, false
	}

	return openCodeMetricRecord{
		Metrics:     codexMetrics{TokensUsed: codexIntPtr(tokensUsed)},
		TimeUpdated: updated,
	}, true
}

func opencodeDBPath() string {
	dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if dataHome == "" {
		dataHome = filepath.Join(userHomeDir(), ".local", "share")
	}
	return filepath.Join(dataHome, "opencode", "opencode.db")
}

func providerContextWindow(model string) (int, bool) {
	model = strings.TrimSpace(strings.ToLower(model))
	if model == "" {
		return 0, false
	}

	if info, ok, err := lookupCodexModelInfo(model); err == nil && ok {
		effectiveWindow := info.ContextWindow
		if info.EffectiveContextWindowPercent > 0 && info.EffectiveContextWindowPercent < 100 {
			effectiveWindow = effectiveWindow * info.EffectiveContextWindowPercent / 100
		}
		if effectiveWindow > 0 {
			return effectiveWindow, true
		}
	}

	if strings.HasPrefix(model, "claude-") {
		return 200000, true
	}
	return 0, false
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

func claudeStringValue(v any) string {
	value, ok := v.(string)
	if !ok {
		return ""
	}
	return value
}

func claudeMapValue(v any) map[string]any {
	value, ok := v.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return value
}

func claudeIntValue(v any) (int, bool) {
	switch value := v.(type) {
	case float64:
		if value < 0 || math.Trunc(value) != value {
			return 0, false
		}
		return int(value), true
	case float32:
		if value < 0 || math.Trunc(float64(value)) != float64(value) {
			return 0, false
		}
		return int(value), true
	case int:
		if value < 0 {
			return 0, false
		}
		return value, true
	case int64:
		if value < 0 {
			return 0, false
		}
		return int(value), true
	case int32:
		if value < 0 {
			return 0, false
		}
		return int(value), true
	case string:
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}
