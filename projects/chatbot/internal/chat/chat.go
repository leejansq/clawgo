/*
 * Chat Bot Management System - Chat Handler
 * 处理聊天逻辑，调用 LLM，支持 MCP 工具
 */

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/leejansq/clawgo/projects/chatbot/internal/manager"
	"github.com/leejansq/clawgo/projects/chatbot/internal/mcp"
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

// ToolProvider 工具提供者接口
type ToolProvider interface {
	GetTools() []*schema.ToolInfo
	CallTool(ctx context.Context, name string, args map[string]any) (string, error)
}

// MCPToolProvider MCP 工具提供者适配器
type MCPToolProvider struct {
	client *mcp.MCPClient
}

// NewMCPToolProvider 创建 MCP 工具提供者
func NewMCPToolProvider(client *mcp.MCPClient) *MCPToolProvider {
	return &MCPToolProvider{client: client}
}

// GetTools 获取 MCP 工具列表
func (p *MCPToolProvider) GetTools() []*schema.ToolInfo {
	mcpTools := p.client.ListTools()
	result := make([]*schema.ToolInfo, 0, len(mcpTools))
	for _, t := range mcpTools {
		result = append(result, &schema.ToolInfo{
			Name: t.Name,
			Desc: t.Description,
		})
	}
	return result
}

// CallTool 调用 MCP 工具
func (p *MCPToolProvider) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	return p.client.CallTool(ctx, name, args)
}

// EinoChatModelAdapter eino 模型适配器，适配 eino 的 ToolCallingChatModel 到 ChatModel 接口
type EinoChatModelAdapter struct {
	cm            model.ToolCallingChatModel
	toolProviders []ToolProvider
}

// NewEinoChatModelAdapter 创建 eino 模型适配器
func NewEinoChatModelAdapter(cm model.ToolCallingChatModel) *EinoChatModelAdapter {
	return &EinoChatModelAdapter{
		cm:            cm,
		toolProviders: make([]ToolProvider, 0),
	}
}

// RegisterToolProvider 注册工具提供者
func (a *EinoChatModelAdapter) RegisterToolProvider(provider ToolProvider) {
	a.toolProviders = append(a.toolProviders, provider)
}

// GetAllTools 获取所有工具
func (a *EinoChatModelAdapter) GetAllTools() []*schema.ToolInfo {
	allTools := make([]*schema.ToolInfo, 0)
	for _, p := range a.toolProviders {
		allTools = append(allTools, p.GetTools()...)
	}
	return allTools
}

// CallTool 调用工具
func (a *EinoChatModelAdapter) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	for _, p := range a.toolProviders {
		tools := p.GetTools()
		for _, t := range tools {
			if t.Name == name {
				return p.CallTool(ctx, name, args)
			}
		}
	}
	return "", fmt.Errorf("tool %s not found", name)
}

// GenerateWithAgent 使用 ReAct Agent 生成回复
func (a *EinoChatModelAdapter) GenerateWithAgent(ctx context.Context, messages []Message) (string, error) {
	// 转换为 eino 消息格式
	input := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		input = append(input, &schema.Message{
			Role:    schema.RoleType(msg.Role),
			Content: msg.Content,
		})
	}

	// 构建 tools node config
	toolsConfig := BuildToolsNodeConfig(a.toolProviders)

	// 创建 ReAct agent
	agent, err := NewReActAgent(ctx, a.cm, toolsConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create ReAct agent: %w", err)
	}

	// 调用 agent
	result, err := agent.Generate(ctx, input)
	if err != nil {
		return "", fmt.Errorf("agent generate error: %w", err)
	}

	return result.Content, nil
}

// Generate 调用 eino LLM 生成回复（支持工具调用循环）
func (a *EinoChatModelAdapter) Generate(ctx context.Context, messages []Message) (string, error) {
	// 转换为 eino 消息格式
	input := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		input = append(input, &schema.Message{
			Role:    schema.RoleType(msg.Role),
			Content: msg.Content,
		})
	}

	// 获取所有工具并绑定到模型
	allTools := a.GetAllTools()
	cmWithTools, err := a.cm.WithTools(allTools)
	if err != nil {
		return "", fmt.Errorf("failed to bind tools: %w", err)
	}

	// 调用 eino LLM
	result, err := cmWithTools.Generate(ctx, input)
	if err != nil {
		return "", fmt.Errorf("LLM generate error: %w", err)
	}
	log.Printf("LLM first response: content=%q, toolCalls=%d", result.Content, len(result.ToolCalls))

	// 处理工具调用循环 (最多 10 次迭代)
	maxIterations := 10
	for len(result.ToolCalls) > 0 && maxIterations > 0 {
		maxIterations--

		// 执行每个工具调用
		for _, tc := range result.ToolCalls {
			// 解析参数
			args := make(map[string]any)
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					log.Printf("Failed to parse tool arguments: %v", err)
					continue
				}
			}

			// 调用工具
			toolResult, err := a.CallTool(ctx, tc.Function.Name, args)
			if err != nil {
				toolResult = fmt.Sprintf("Error: %v", err)
			}

			// 记录工具调用
			log.Printf("Tool call: id=%s, name=%s, args=%v, result=%s", tc.ID, tc.Function.Name, args, toolResult)

			// 构造 tool result 消息
			toolResultMsg := &schema.Message{
				Role:       schema.RoleType("tool"),
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    toolResult,
			}
			log.Printf("Tool result message: role=%s, toolCallID=%s, toolName=%s, content=%q", toolResultMsg.Role, toolResultMsg.ToolCallID, toolResultMsg.ToolName, toolResultMsg.Content)
			input = append(input, toolResultMsg)
		}

		// 继续 LLM 调用
		log.Printf("Calling LLM again after tool calls, input messages count: %d", len(input))
		// 打印 input 中最后的几条消息（tool 结果）
		for i := len(input) - 3; i < len(input); i++ {
			if i >= 0 {
				msg := input[i]
				log.Printf("  input[%d]: role=%s, toolCallID=%s, content=%q", i, msg.Role, msg.ToolCallID, msg.Content)
			}
		}
		result, err = cmWithTools.Generate(ctx, input)
		if err != nil {
			log.Printf("LLM generate error in tool loop: %v", err)
			return "", fmt.Errorf("LLM generate error in tool loop: %w", err)
		}
		log.Printf("LLM response after tool: content=%q, toolCalls=%d", result.Content, len(result.ToolCalls))
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

	// 尝试使用 ReAct Agent
	if adapter, ok := h.cm.(*EinoChatModelAdapter); ok {
		return adapter.GenerateWithAgent(ctx, input)
	}

	// 回退到普通 Generate
	return h.cm.Generate(ctx, input)
}

// mockReply 生成模拟回复（当没有配置 LLM 时使用）
func (h *ChatHandler) mockReply(userMessage, label string) string {
	return fmt.Sprintf("[%s] 我收到了您的消息: %s\n\n这是一条模拟回复，因为当前没有配置 LLM。\n要启用真正的 AI 回复，请配置 MODEL_TYPE, ARK_API_KEY 或 OPENAI_API_KEY 环境变量。", label, userMessage)
}
