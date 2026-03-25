/*
 * Chat Bot Management System - SQLite Storage
 * 存储子 Agent 的会话和消息记录
 */

package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteStore SQLite 存储
type SQLiteStore struct {
	db *sql.DB
}

// Message 消息结构
type Message struct {
	ID        int64     `json:"id"`
	SessionKey string   `json:"sessionKey"`
	Role      string    `json:"role"` // user, assistant
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

// Session 会话结构
type Session struct {
	ID          int64     `json:"id"`
	SessionKey  string    `json:"sessionKey"`
	Label       string    `json:"label"`
	SystemPrompt string   `json:"systemPrompt"`
	Status      string    `json:"status"` // active, closed
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// NewSQLiteStore 创建 SQLite 存储
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.initTables(); err != nil {
		return nil, fmt.Errorf("failed to init tables: %w", err)
	}

	return store, nil
}

// initTables 初始化表
func (s *SQLiteStore) initTables() error {
	// 创建 sessions 表
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_key TEXT UNIQUE NOT NULL,
			label TEXT NOT NULL,
			system_prompt TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create sessions table: %w", err)
	}

	// 创建 messages 表
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_key TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			FOREIGN KEY (session_key) REFERENCES sessions(session_key)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create messages table: %w", err)
	}

	// 创建索引
	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_key)`)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	return nil
}

// Close 关闭数据库
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// ============================================================================
// Session Operations
// ============================================================================

// CreateSession 创建会话
func (s *SQLiteStore) CreateSession(sessionKey, label, systemPrompt string) (*Session, error) {
	now := time.Now()
	_, err := s.db.Exec(
		`INSERT INTO sessions (session_key, label, system_prompt, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)`,
		sessionKey, label, systemPrompt, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &Session{
		SessionKey:   sessionKey,
		Label:        label,
		SystemPrompt: systemPrompt,
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// GetSession 获取会话
func (s *SQLiteStore) GetSession(sessionKey string) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT id, session_key, label, system_prompt, status, created_at, updated_at FROM sessions WHERE session_key = ?`,
		sessionKey,
	)

	var session Session
	err := row.Scan(&session.ID, &session.SessionKey, &session.Label, &session.SystemPrompt, &session.Status, &session.CreatedAt, &session.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return &session, nil
}

// ListSessions 列出所有会话
func (s *SQLiteStore) ListSessions() ([]*Session, error) {
	rows, err := s.db.Query(
		`SELECT id, session_key, label, system_prompt, status, created_at, updated_at FROM sessions ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var session Session
		if err := rows.Scan(&session.ID, &session.SessionKey, &session.Label, &session.SystemPrompt, &session.Status, &session.CreatedAt, &session.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		sessions = append(sessions, &session)
	}

	return sessions, nil
}

// UpdateSessionStatus 更新会话状态
func (s *SQLiteStore) UpdateSessionStatus(sessionKey, status string) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET status = ?, updated_at = ? WHERE session_key = ?`,
		status, time.Now(), sessionKey,
	)
	return err
}

// DeleteSession 删除会话及其消息
func (s *SQLiteStore) DeleteSession(sessionKey string) error {
	// 删除消息
	_, err := s.db.Exec(`DELETE FROM messages WHERE session_key = ?`, sessionKey)
	if err != nil {
		return fmt.Errorf("failed to delete messages: %w", err)
	}

	// 删除会话
	_, err = s.db.Exec(`DELETE FROM sessions WHERE session_key = ?`, sessionKey)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// ============================================================================
// Message Operations
// ============================================================================

// AddMessage 添加消息
func (s *SQLiteStore) AddMessage(sessionKey, role, content string) (*Message, error) {
	now := time.Now()
	result, err := s.db.Exec(
		`INSERT INTO messages (session_key, role, content, created_at) VALUES (?, ?, ?, ?)`,
		sessionKey, role, content, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to add message: %w", err)
	}

	id, _ := result.LastInsertId()

	// 更新会话更新时间
	s.db.Exec(`UPDATE sessions SET updated_at = ? WHERE session_key = ?`, now, sessionKey)

	return &Message{
		ID:         id,
		SessionKey: sessionKey,
		Role:       role,
		Content:    content,
		CreatedAt:  now,
	}, nil
}

// GetMessages 获取会话的所有消息
func (s *SQLiteStore) GetMessages(sessionKey string) ([]*Message, error) {
	rows, err := s.db.Query(
		`SELECT id, session_key, role, content, created_at FROM messages WHERE session_key = ? ORDER BY created_at ASC`,
		sessionKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.SessionKey, &msg.Role, &msg.Content, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, &msg)
	}

	return messages, nil
}
