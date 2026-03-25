/*
 * Chat Bot Management System - Chat Handler
 * 处理聊天逻辑，调用 LLM
 */

package chat

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/leejansq/clawgo/projects/chatbot/internal/manager"
)

// Message LLM 消息格式
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatModel LLM 模型接口
type ChatModel interface {
	Generate(ctx context.Context, messages []Message) (string, error)
}

// EinoChatModelAdapter eino 模型适配器，适配 eino 的 ToolCallingChatModel 到 ChatModel 接口
type EinoChatModelAdapter struct {
	cm model.ToolCallingChatModel
}

// NewEinoChatModelAdapter 创建 eino 模型适配器
func NewEinoChatModelAdapter(cm model.ToolCallingChatModel) *EinoChatModelAdapter {
	return &EinoChatModelAdapter{cm: cm}
}

// Generate 调用 eino LLM 生成回复
func (a *EinoChatModelAdapter) Generate(ctx context.Context, messages []Message) (string, error) {
	// 转换为 eino 消息格式
	input := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		input = append(input, &schema.Message{
			Role:    schema.RoleType(msg.Role),
			Content: msg.Content,
		})
	}

	// 调用 eino LLM
	result, err := a.cm.Generate(ctx, input)
	if err != nil {
		return "", fmt.Errorf("LLM generate error: %w", err)
	}

	return result.Content, nil
}

// ChatHandler 聊天处理器
type ChatHandler struct {
	manager *manager.Manager
	cm      ChatModel
}

// NewChatHandler 创建聊天处理器
func NewChatHandler(manager *manager.Manager, cm ChatModel) *ChatHandler {
	return &ChatHandler{
		manager: manager,
		cm:      cm,
	}
}

// HandleMessage 处理用户消息，返回助手回复
func (h *ChatHandler) HandleMessage(ctx context.Context, sessionKey, userMessage string) (string, error) {
	// 获取会话
	chatSession, err := h.manager.GetSession(sessionKey)
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}
	if chatSession == nil {
		return "", fmt.Errorf("session not found: %s", sessionKey)
	}

	// 保存用户消息
	if _, err := h.manager.AddMessage(sessionKey, "user", userMessage); err != nil {
		log.Printf("Failed to save user message: %v", err)
	}

	// 获取用于 LLM 的消息列表
	llmMessages := chatSession.GetMessagesForLLM()

	// 调用 LLM
	var assistantReply string
	if h.cm == nil {
		// 如果没有配置 LLM，使用模拟回复
		assistantReply = h.mockReply(userMessage, chatSession.Label)
	} else {
		// 使用 LLM 生成回复
		reply, err := h.callLLM(ctx, llmMessages)
		if err != nil {
			return "", fmt.Errorf("failed to call LLM: %w", err)
		}
		assistantReply = reply
	}

	// 保存助手回复
	if _, err := h.manager.AddMessage(sessionKey, "assistant", assistantReply); err != nil {
		log.Printf("Failed to save assistant message: %v", err)
	}

	return assistantReply, nil
}

// callLLM 调用 LLM
func (h *ChatHandler) callLLM(ctx context.Context, messages []map[string]string) (string, error) {
	// 构建输入
	input := make([]Message, 0, len(messages))
	for _, msg := range messages {
		input = append(input, Message{
			Role:    msg["role"],
			Content: msg["content"],
		})
	}

	// 调用 LLM
	return h.cm.Generate(ctx, input)
}

// mockReply 生成模拟回复（当没有配置 LLM 时使用）
func (h *ChatHandler) mockReply(userMessage, label string) string {
	return fmt.Sprintf("[%s] 我收到了您的消息: %s\n\n这是一条模拟回复，因为当前没有配置 LLM。\n要启用真正的 AI 回复，请配置 MODEL_TYPE, ARK_API_KEY 或 OPENAI_API_KEY 环境变量。", label, userMessage)
}
