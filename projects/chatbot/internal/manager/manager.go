/*
 * Chat Bot Management System - Manager
 * 主 Agent 管理器
 */

package manager

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/leejansq/clawgo/projects/chatbot/internal/session"
	"github.com/leejansq/clawgo/projects/chatbot/internal/store"
)

// Manager 主 Agent 管理器
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*session.ChatSession
	store    *store.SQLiteStore
}

// NewManager 创建管理器
func NewManager(store *store.SQLiteStore) *Manager {
	return &Manager{
		sessions: make(map[string]*session.ChatSession),
		store:    store,
	}
}

// CreateSession 创建子 Agent 会话
func (m *Manager) CreateSession(label, systemPrompt string) (*session.ChatSession, string, error) {
	// 生成唯一会话 key
	sessionKey := fmt.Sprintf("chatbot:%s:%s", label, uuid.New().String())

	// 创建会话对象
	chatSession := session.NewChatSession(sessionKey, label, systemPrompt)

	// 持久化到数据库
	if _, err := m.store.CreateSession(sessionKey, label, systemPrompt); err != nil {
		return nil, "", fmt.Errorf("failed to create session in store: %w", err)
	}

	// 注册到内存
	m.mu.Lock()
	m.sessions[sessionKey] = chatSession
	m.mu.Unlock()

	return chatSession, sessionKey, nil
}

// GetSession 获取会话
func (m *Manager) GetSession(sessionKey string) (*session.ChatSession, error) {
	// 先从内存获取
	m.mu.RLock()
	if chatSession, ok := m.sessions[sessionKey]; ok {
		m.mu.RUnlock()
		return chatSession, nil
	}
	m.mu.RUnlock()

	// 从数据库加载
	dbSession, err := m.store.GetSession(sessionKey)
	if err != nil {
		return nil, err
	}
	if dbSession == nil {
		return nil, nil
	}

	// 加载消息
	messages, err := m.store.GetMessages(sessionKey)
	if err != nil {
		return nil, err
	}

	// 构建会话对象
	chatSession := session.NewChatSession(dbSession.SessionKey, dbSession.Label, dbSession.SystemPrompt)
	chatSession.UpdateStatus(dbSession.Status)
	for _, msg := range messages {
		chatSession.AddMessage(msg.Role, msg.Content)
	}

	// 缓存到内存
	m.mu.Lock()
	m.sessions[sessionKey] = chatSession
	m.mu.Unlock()

	return chatSession, nil
}

// ListSessions 列出所有会话
func (m *Manager) ListSessions() ([]*session.ChatSession, error) {
	// 从数据库获取所有会话
	dbSessions, err := m.store.ListSessions()
	if err != nil {
		return nil, err
	}

	sessions := make([]*session.ChatSession, 0, len(dbSessions))
	for _, dbSession := range dbSessions {
		chatSession, err := m.GetSession(dbSession.SessionKey)
		if err != nil {
			continue
		}
		if chatSession != nil {
			sessions = append(sessions, chatSession)
		}
	}

	return sessions, nil
}

// DeleteSession 删除会话
func (m *Manager) DeleteSession(sessionKey string) error {
	// 从内存移除
	m.mu.Lock()
	delete(m.sessions, sessionKey)
	m.mu.Unlock()

	// 从数据库删除
	return m.store.DeleteSession(sessionKey)
}

// AddMessage 添加消息到会话
func (m *Manager) AddMessage(sessionKey, role, content string) (*store.Message, error) {
	// 添加到数据库
	msg, err := m.store.AddMessage(sessionKey, role, content)
	if err != nil {
		return nil, err
	}

	// 添加到内存会话
	m.mu.RLock()
	chatSession, ok := m.sessions[sessionKey]
	m.mu.RUnlock()

	if ok {
		chatSession.AddMessage(role, content)
	}

	return msg, nil
}

// GetMessages 获取会话消息
func (m *Manager) GetMessages(sessionKey string) ([]*store.Message, error) {
	return m.store.GetMessages(sessionKey)
}

// SessionExists 检查会话是否存在
func (m *Manager) SessionExists(sessionKey string) bool {
	_, err := m.GetSession(sessionKey)
	return err == nil && sessionKey != ""
}
