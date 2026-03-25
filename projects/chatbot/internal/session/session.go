/*
 * Chat Bot Management System - Chat Session
 * 子 Agent 会话结构
 */

package session

import (
	"sync"
	"time"
)

// Message 聊天消息
type Message struct {
	ID        int64     `json:"id"`
	Role      string    `json:"role"` // user, assistant
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

// ChatSession 子 Agent 会话
type ChatSession struct {
	SessionKey   string     `json:"sessionKey"`
	Label        string     `json:"label"`
	SystemPrompt string     `json:"systemPrompt"`
	Status       string     `json:"status"` // active, closed
	Messages     []*Message `json:"messages"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	mu           sync.RWMutex
}

// NewChatSession 创建新会话
func NewChatSession(sessionKey, label, systemPrompt string) *ChatSession {
	now := time.Now()
	return &ChatSession{
		SessionKey:   sessionKey,
		Label:        label,
		SystemPrompt: systemPrompt,
		Status:       "active",
		Messages:     make([]*Message, 0),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// AddMessage 添加消息
func (s *ChatSession) AddMessage(role, content string) *Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	msg := &Message{
		ID:        int64(len(s.Messages) + 1),
		Role:      role,
		Content:   content,
		CreatedAt: now,
	}
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = now

	return msg
}

// GetMessages 获取所有消息
func (s *ChatSession) GetMessages() []*Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 返回副本
	messages := make([]*Message, len(s.Messages))
	copy(messages, s.Messages)
	return messages
}

// GetMessagesForLLM 获取用于 LLM 调用格式的消息
func (s *ChatSession) GetMessagesForLLM() []map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	messages := make([]map[string]string, 0, len(s.Messages)+1)

	// 添加系统提示
	if s.SystemPrompt != "" {
		messages = append(messages, map[string]string{
			"role":    "system",
			"content": s.SystemPrompt,
		})
	}

	// 添加历史消息
	for _, msg := range s.Messages {
		messages = append(messages, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	return messages
}

// UpdateStatus 更新状态
func (s *ChatSession) UpdateStatus(status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
	s.UpdatedAt = time.Now()
}

// GetStatus 获取状态
func (s *ChatSession) GetStatus() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Status
}
