//go:build cgo
// +build cgo

package storage

import (
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	currentSchemaVersion = 1
	maxRetries           = 15
	baseRetryDelay       = 20 * time.Millisecond
	maxRetryDelay        = 150 * time.Millisecond
)

type SessionDB struct {
	db   *sql.DB
	path string
}

func NewSessionDB(homeDir string) (*SessionDB, error) {
	dbPath := filepath.Join(homeDir, "sessions.db")

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	sdb := &SessionDB{db: db, path: dbPath}

	if err := sdb.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err := sdb.setPermissions(); err != nil {
		db.Close()
		return nil, fmt.Errorf("set permissions: %w", err)
	}

	return sdb, nil
}

func (s *SessionDB) setPermissions() error {
	files := []string{s.path, s.path + "-wal", s.path + "-shm"}
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			if err := os.Chmod(f, 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *SessionDB) migrate() error {
	var version int
	err := s.db.QueryRow("PRAGMA user_version").Scan(&version)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	if version >= currentSchemaVersion {
		return nil
	}

	hasFTS5 := s.checkFTS5()

	baseMigration := `
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			source TEXT NOT NULL DEFAULT 'cli',
			user_id TEXT,
			model TEXT NOT NULL,
			model_config TEXT,
			system_prompt TEXT,
			parent_session_id TEXT,
			started_at REAL NOT NULL,
			ended_at REAL,
			end_reason TEXT,
			message_count INTEGER NOT NULL DEFAULT 0,
			tool_call_count INTEGER NOT NULL DEFAULT 0,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			cache_read_tokens INTEGER NOT NULL DEFAULT 0,
			cache_write_tokens INTEGER NOT NULL DEFAULT 0,
			reasoning_tokens INTEGER NOT NULL DEFAULT 0,
			billing_provider TEXT,
			billing_base_url TEXT,
			billing_mode TEXT,
			estimated_cost_usd REAL NOT NULL DEFAULT 0,
			actual_cost_usd REAL NOT NULL DEFAULT 0,
			cost_status TEXT,
			cost_source TEXT,
			pricing_version TEXT,
			title TEXT,
			FOREIGN KEY (parent_session_id) REFERENCES sessions(id)
		);

		CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_title ON sessions(title) WHERE title IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_sessions_source ON sessions(source);
		CREATE INDEX IF NOT EXISTS idx_sessions_started ON sessions(started_at);

		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT,
			tool_call_id TEXT,
			tool_calls TEXT,
			tool_name TEXT,
			timestamp REAL NOT NULL,
			token_count INTEGER,
			finish_reason TEXT,
			reasoning TEXT,
			reasoning_details TEXT,
			FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id);
		CREATE INDEX IF NOT EXISTS idx_messages_role ON messages(role);
	`

	ftsMigration := `
		CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
			content,
			content='messages',
			content_rowid='id'
		);

		CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
			INSERT INTO messages_fts(rowid, content) VALUES (new.id, new.content);
		END;

		CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, content) VALUES('delete', old.id, old.content);
		END;

		CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, content) VALUES('delete', old.id, old.content);
			INSERT INTO messages_fts(rowid, content) VALUES (new.id, new.content);
		END;
	`

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer tx.Rollback()

	for v := version; v < currentSchemaVersion; v++ {
		if _, err := tx.Exec(baseMigration); err != nil {
			return fmt.Errorf("execute migration v%d base: %w", v, err)
		}
		if hasFTS5 {
			if _, err := tx.Exec(ftsMigration); err != nil {
				return fmt.Errorf("execute migration v%d fts5: %w", v, err)
			}
		}
	}

	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", currentSchemaVersion)); err != nil {
		return fmt.Errorf("set user_version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}

	return nil
}

func (s *SessionDB) checkFTS5() bool {
	rows, err := s.db.Query("PRAGMA compile_options")
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var opt string
		if err := rows.Scan(&opt); err != nil {
			continue
		}
		if opt == "ENABLE_FTS5" {
			return true
		}
	}
	return false
}

func (s *SessionDB) retry(fn func() error) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if err.Error() != "database is locked" {
			return err
		}
		delay := baseRetryDelay + time.Duration(rand.Int63n(int64(maxRetryDelay-baseRetryDelay)))
		time.Sleep(delay)
	}
	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (s *SessionDB) CreateSession(id, source, userID, model string, modelConfig, systemPrompt string) error {
	return s.retry(func() error {
		_, err := s.db.Exec(`
			INSERT INTO sessions (id, source, user_id, model, model_config, system_prompt, started_at, message_count, tool_call_count)
			VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0)
		`, id, source, userID, model, modelConfig, systemPrompt, float64(time.Now().Unix()))
		return err
	})
}

func (s *SessionDB) GetSession(id string) (map[string]interface{}, error) {
	row := s.db.QueryRow(`
		SELECT id, source, user_id, model, model_config, system_prompt, parent_session_id,
		       started_at, ended_at, end_reason, message_count, tool_call_count,
		       input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		       reasoning_tokens, estimated_cost_usd, actual_cost_usd, title
		FROM sessions WHERE id = ?
	`, id)

	var sessID, source, userID, model, modelConfig, systemPrompt, parentID, endReason, title sql.NullString
	var startedAt, endedAt sql.NullFloat64
	var msgCount, toolCount, inputTokens, outputTokens, cacheRead, cacheWrite, reasoningTokens int
	var estCost, actualCost float64

	err := row.Scan(
		&sessID, &source, &userID, &model, &modelConfig, &systemPrompt, &parentID,
		&startedAt, &endedAt, &endReason, &msgCount, &toolCount,
		&inputTokens, &outputTokens, &cacheRead, &cacheWrite, &reasoningTokens,
		&estCost, &actualCost, &title,
	)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"id":                 sessID.String,
		"source":             source.String,
		"user_id":            userID.String,
		"model":              model.String,
		"model_config":       modelConfig.String,
		"system_prompt":      systemPrompt.String,
		"parent_session_id":  parentID.String,
		"started_at":         startedAt.Float64,
		"ended_at":           endedAt.Float64,
		"end_reason":         endReason.String,
		"message_count":      msgCount,
		"tool_call_count":    toolCount,
		"input_tokens":       inputTokens,
		"output_tokens":      outputTokens,
		"cache_read_tokens":  cacheRead,
		"cache_write_tokens": cacheWrite,
		"reasoning_tokens":   reasoningTokens,
		"estimated_cost_usd": estCost,
		"actual_cost_usd":    actualCost,
		"title":              title.String,
	}

	return result, nil
}

func (s *SessionDB) UpdateSessionMetrics(id string, inputTokens, outputTokens, cacheRead, cacheWrite, reasoningTokens int, cost float64) error {
	return s.retry(func() error {
		_, err := s.db.Exec(`
			UPDATE sessions SET
				input_tokens = input_tokens + ?,
				output_tokens = output_tokens + ?,
				cache_read_tokens = cache_read_tokens + ?,
				cache_write_tokens = cache_write_tokens + ?,
				reasoning_tokens = reasoning_tokens + ?,
				estimated_cost_usd = estimated_cost_usd + ?
			WHERE id = ?
		`, inputTokens, outputTokens, cacheRead, cacheWrite, reasoningTokens, cost, id)
		return err
	})
}

func (s *SessionDB) EndSession(id, reason string) error {
	return s.retry(func() error {
		_, err := s.db.Exec(`
			UPDATE sessions SET ended_at = ?, end_reason = ? WHERE id = ?
		`, float64(time.Now().Unix()), reason, id)
		return err
	})
}

func (s *SessionDB) AddMessage(sessionID, role, content, toolCallID, toolCalls, toolName, reasoning, reasoningDetails string, tokenCount int, finishReason string) error {
	return s.retry(func() error {
		_, err := s.db.Exec(`
			INSERT INTO messages (session_id, role, content, tool_call_id, tool_calls, tool_name, timestamp, token_count, finish_reason, reasoning, reasoning_details)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, sessionID, role, content, toolCallID, toolCalls, toolName, float64(time.Now().Unix()), tokenCount, finishReason, reasoning, reasoningDetails)
		if err != nil {
			return err
		}
		_, err = s.db.Exec(`UPDATE sessions SET message_count = message_count + 1 WHERE id = ?`, sessionID)
		return err
	})
}

func (s *SessionDB) GetMessages(sessionID string) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(`
		SELECT id, role, content, tool_call_id, tool_calls, tool_name, timestamp, token_count, finish_reason, reasoning, reasoning_details
		FROM messages WHERE session_id = ? ORDER BY id ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []map[string]interface{}
	for rows.Next() {
		var id int
		var role, content, toolCallID, toolCalls, toolName, finishReason, reasoning, reasoningDetails sql.NullString
		var timestamp sql.NullFloat64
		var tokenCount sql.NullInt64

		if err := rows.Scan(&id, &role, &content, &toolCallID, &toolCalls, &toolName, &timestamp, &tokenCount, &finishReason, &reasoning, &reasoningDetails); err != nil {
			return nil, err
		}

		msg := map[string]interface{}{
			"role":              role.String,
			"content":           content.String,
			"tool_call_id":      toolCallID.String,
			"tool_calls":        toolCalls.String,
			"tool_name":         toolName.String,
			"timestamp":         timestamp.Float64,
			"token_count":       tokenCount.Int64,
			"finish_reason":     finishReason.String,
			"reasoning":         reasoning.String,
			"reasoning_details": reasoningDetails.String,
		}
		messages = append(messages, msg)
	}

	return messages, rows.Err()
}

func (s *SessionDB) SearchMessages(query string) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(`
		SELECT m.id, m.session_id, m.role, m.content, m.timestamp
		FROM messages_fts f
		JOIN messages m ON m.id = f.rowid
		WHERE messages_fts MATCH ?
		ORDER BY rank LIMIT 20
	`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int
		var sessionID, role, content sql.NullString
		var timestamp sql.NullFloat64

		if err := rows.Scan(&id, &sessionID, &role, &content, &timestamp); err != nil {
			return nil, err
		}

		results = append(results, map[string]interface{}{
			"id":         id,
			"session_id": sessionID.String,
			"role":       role.String,
			"content":    content.String,
			"timestamp":  timestamp.Float64,
		})
	}

	return results, rows.Err()
}

func (s *SessionDB) IncrementToolCalls(sessionID string) error {
	return s.retry(func() error {
		_, err := s.db.Exec(`UPDATE sessions SET tool_call_count = tool_call_count + 1 WHERE id = ?`, sessionID)
		return err
	})
}

func (s *SessionDB) Close() error {
	return s.db.Close()
}
