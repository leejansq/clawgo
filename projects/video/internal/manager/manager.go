/*
 * Video Script Generation - Manager
 * 子智能体管理器，支持工具调用
 */

package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/leejansq/clawgo/projects/video/pkg/prompt"
	videoschema "github.com/leejansq/clawgo/projects/video/pkg/schema"
)

// ============================================================================
// ToolProvider 接口
// ============================================================================

// ToolProvider 工具提供者接口
type ToolProvider interface {
	GetTools() []*schema.ToolInfo
	CallTool(ctx context.Context, name string, args map[string]any) (string, error)
}

// ============================================================================
// Manager - 子智能体管理器
// ============================================================================

// AgentType 子智能体类型
type AgentType string

const (
	AgentTypeResearcher   AgentType = "researcher"
	AgentTypeScriptwriter AgentType = "scriptwriter"
	AgentTypeCritic       AgentType = "critic"
)

// Session 子智能体会话
type Session struct {
	SessionKey   string
	AgentType    AgentType
	SystemPrompt string
	Status       string // pending, running, completed, error
	Input        string
	Output       string
	Error        string
	CreatedAt    time.Time
	CompletedAt  *time.Time
}

// Manager 子智能体管理器
type Manager struct {
	mu            sync.RWMutex
	sessions      map[string]*Session
	cm            model.ToolCallingChatModel
	toolProviders []ToolProvider
}

// NewManager 创建管理器（使用 ToolCallingChatModel）
func NewManager(cm model.ToolCallingChatModel) *Manager {
	return &Manager{
		sessions:      make(map[string]*Session),
		cm:            cm,
		toolProviders: make([]ToolProvider, 0),
	}
}

// RegisterToolProvider 注册工具提供者
func (m *Manager) RegisterToolProvider(provider ToolProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolProviders = append(m.toolProviders, provider)
}

// GetAllTools 获取所有工具
func (m *Manager) GetAllTools() []*schema.ToolInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	allTools := make([]*schema.ToolInfo, 0)
	for _, p := range m.toolProviders {
		allTools = append(allTools, p.GetTools()...)
	}
	return allTools
}

// CallTool 调用工具
func (m *Manager) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.toolProviders {
		tools := p.GetTools()
		for _, t := range tools {
			if t.Name == name {
				return p.CallTool(ctx, name, args)
			}
		}
	}
	return "", fmt.Errorf("tool %s not found", name)
}

// CreateSession 创建子智能体会话
func (m *Manager) CreateSession(agentType AgentType, input string) (*Session, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessionKey := fmt.Sprintf("video:%s:%s", agentType, uuid.New().String())

	// 获取对应的 system prompt
	systemPrompt := getSystemPrompt(agentType)

	session := &Session{
		SessionKey:   sessionKey,
		AgentType:    agentType,
		SystemPrompt: systemPrompt,
		Status:       "pending",
		Input:        input,
		CreatedAt:    time.Now(),
	}

	m.sessions[sessionKey] = session
	return session, sessionKey, nil
}

// GetSession 获取会话
func (m *Manager) GetSession(sessionKey string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionKey]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionKey)
	}
	return session, nil
}

// UpdateSessionStatus 更新会话状态
func (m *Manager) UpdateSessionStatus(sessionKey, status, output, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionKey]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionKey)
	}

	session.Status = status
	if output != "" {
		session.Output = output
	}
	if errMsg != "" {
		session.Error = errMsg
	}
	if status == "completed" || status == "error" {
		now := time.Now()
		session.CompletedAt = &now
	}

	return nil
}

// ListSessions 列出所有会话
func (m *Manager) ListSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

// GenerateWithTools 调用 LLM 生成（支持工具调用循环）
func (m *Manager) GenerateWithTools(ctx context.Context, systemPrompt, userInput string) (string, error) {
	m.mu.RLock()
	cm := m.cm
	m.mu.RUnlock()

	if cm == nil {
		return "", fmt.Errorf("chat model not set")
	}

	// 获取所有工具并绑定到模型
	allTools := m.GetAllTools()
	cmWithTools, err := cm.WithTools(allTools)
	if err != nil {
		return "", fmt.Errorf("failed to bind tools: %w", err)
	}

	// 构建消息
	input := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(userInput),
	}

	// 打印 LLM 输入日志
	log.Printf("=== LLM INPUT ===")
	log.Printf("System: %s", systemPrompt)
	log.Printf("User: %s", userInput)
	// log.Printf("Tools available: %d", len(allTools))
	// for _, t := range allTools {
	// 	log.Printf("  - %s: %s", t.Name, t.Desc)
	// }
	log.Printf("=================")

	// 调用 LLM（带重试）
	result, err := m.callLLMWithRetry(ctx, cmWithTools, input)
	if err != nil {
		return "", fmt.Errorf("LLM generation failed: %w", err)
	}
	log.Printf("=== LLM OUTPUT ===")
	log.Printf("Content: %s", result.Content)
	log.Printf("ToolCalls: %d", len(result.ToolCalls))
	for _, tc := range result.ToolCalls {
		log.Printf("  - %s(%s)", tc.Function.Name, tc.Function.Arguments)
	}
	log.Printf("==================")

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

			log.Printf(">>> ToolCall ID: %s, Name: %s", tc.ID, tc.Function.Name)
			log.Printf(">>> ToolCall Arguments: %s", tc.Function.Arguments)

			// 调用工具
			toolResult, err := m.CallTool(ctx, tc.Function.Name, args)
			if err != nil {
				toolResult = fmt.Sprintf("Error: %v", err)
			}

			log.Printf("Tool call: name=%s, args=%v, result=%s", tc.Function.Name, args, toolResult)

			// 构造 tool result 消息
			toolResultMsg := &schema.Message{
				Role:       schema.RoleType("tool"),
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    toolResult,
			}
			log.Printf(">>> Sending tool result: Role=%s, ToolCallID=%s, ToolName=%s",
				toolResultMsg.Role, toolResultMsg.ToolCallID, toolResultMsg.ToolName)
			input = append(input, toolResultMsg)
		}

		// 继续 LLM 调用（带重试）
		log.Printf("=== LLM CALL (after %d tool results) ===", len(result.ToolCalls))
		// 打印输入消息中的最后几条（tool results）
		for i := len(input) - len(result.ToolCalls)*2 - 3; i < len(input); i++ {
			if i >= 0 {
				msg := input[i]
				log.Printf("  input[%d]: role=%s, toolCallID=%s, content_len=%d", i, msg.Role, msg.ToolCallID, len(msg.Content))
			}
		}
		result, err = m.callLLMWithRetry(ctx, cmWithTools, input)
		if err != nil {
			return "", fmt.Errorf("LLM generate error in tool loop: %w", err)
		}
		log.Printf("=== LLM OUTPUT (after tools) ===")
		log.Printf("Content: %s", result.Content)
		log.Printf("ToolCalls: %d", len(result.ToolCalls))
		for _, tc := range result.ToolCalls {
			log.Printf("  - %s(%s)", tc.Function.Name, tc.Function.Arguments)
		}
		log.Printf("===============================")
	}

	return result.Content, nil
}

// callLLMWithRetry 调用 LLM（带重试）
func (m *Manager) callLLMWithRetry(ctx context.Context, cm model.ToolCallingChatModel, input []*schema.Message) (*schema.Message, error) {

	maxRetries := 3
	baseDelay := 2 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		result, err := cm.Generate(ctx, input)
		if err == nil {
			return result, nil
		}

		// 检查是否是 529 错误（服务负载高）
		errMsg := err.Error()
		isRetryable := false
		if strings.Contains(errMsg, "529") || strings.Contains(errMsg, "status code: 529") {
			isRetryable = true
			log.Printf("LLM service overloaded (529), retrying in %v... (attempt %d/%d)", baseDelay*time.Duration(1<<attempt), attempt+1, maxRetries)
		} else if strings.Contains(errMsg, "status code: 429") || strings.Contains(errMsg, "rate limit") {
			isRetryable = true
			log.Printf("LLM rate limited, retrying in %v... (attempt %d/%d)", baseDelay*time.Duration(1<<attempt), attempt+1, maxRetries)
		}

		if !isRetryable || attempt == maxRetries-1 {
			return nil, err
		}

		// 指数退避
		delay := baseDelay * time.Duration(1<<attempt)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			// 继续重试
		}
	}

	return nil, fmt.Errorf("max retries exceeded")
}

// ============================================================================
// 子智能体执行方法
// ============================================================================

// ExecuteResearcher 执行研究员任务
func (m *Manager) ExecuteResearcher(ctx context.Context, input *videoschema.ResearcherInput) (*videoschema.ResearcherOutput, error) {
	session, sessionKey, err := m.CreateSession(AgentTypeResearcher, "")
	if err != nil {
		return nil, err
	}

	m.UpdateSessionStatus(sessionKey, "running", "", "")

	// 构建 prompt
	userPrompt := buildResearcherPrompt(input)

	// 调用 LLM（支持工具调用）
	output, err := m.GenerateWithTools(ctx, session.SystemPrompt, userPrompt)
	if err != nil {
		m.UpdateSessionStatus(sessionKey, "error", "", err.Error())
		return nil, err
	}

	m.UpdateSessionStatus(sessionKey, "completed", output, "")

	// 解析输出
	var result videoschema.ResearcherOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		// 如果不是 JSON，返回纯文本结果
		result.Overview = output
	}

	return &result, nil
}

// ExecuteScriptwriter 执行编剧任务
func (m *Manager) ExecuteScriptwriter(ctx context.Context, input *videoschema.ScriptwriterInput) (*videoschema.VideoScript, error) {
	session, sessionKey, err := m.CreateSession(AgentTypeScriptwriter, "")
	if err != nil {
		return nil, err
	}

	m.UpdateSessionStatus(sessionKey, "running", "", "")

	// 构建 prompt
	userPrompt := prompt.BuildScriptwriterPrompt(input)

	// 调用 LLM
	output, err := m.GenerateWithTools(ctx, session.SystemPrompt, userPrompt)
	if err != nil {
		m.UpdateSessionStatus(sessionKey, "error", "", err.Error())
		return nil, err
	}

	m.UpdateSessionStatus(sessionKey, "completed", output, "")

	// 解析输出，尝试提取 JSON
	script := parseVideoScript(output)
	return script, nil
}

// ExecuteCritic 执行评论家任务
func (m *Manager) ExecuteCritic(ctx context.Context, input *videoschema.CriticInput) (*videoschema.CriticOutput, error) {
	session, sessionKey, err := m.CreateSession(AgentTypeCritic, "")
	if err != nil {
		return nil, err
	}

	m.UpdateSessionStatus(sessionKey, "running", "", "")

	// 构建 prompt
	userPrompt := prompt.BuildCriticPrompt(input)

	// 调用 LLM
	output, err := m.GenerateWithTools(ctx, session.SystemPrompt, userPrompt)
	if err != nil {
		m.UpdateSessionStatus(sessionKey, "error", "", err.Error())
		return nil, err
	}

	m.UpdateSessionStatus(sessionKey, "completed", output, "")

	// 解析输出
	var result videoschema.CriticOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		// 如果不是 JSON，尝试提取评分
		result = parseCriticOutput(output)
	}

	return &result, nil
}

// ============================================================================
// Prompt 构建函数
// ============================================================================

// getSystemPrompt 获取各智能体的 system prompt
func getSystemPrompt(agentType AgentType) string {
	switch agentType {
	case AgentTypeResearcher:
		return prompt.ResearcherSystemPrompt
	case AgentTypeScriptwriter:
		return prompt.ScriptwriterSystemPrompt
	case AgentTypeCritic:
		return prompt.CriticSystemPrompt
	default:
		return ""
	}
}

// buildResearcherPrompt 构建研究员 prompt
func buildResearcherPrompt(input *videoschema.ResearcherInput) string {
	aspects := ""
	if len(input.Aspects) > 0 {
		aspects = "重点关注以下方面：\n"
		for _, a := range input.Aspects {
			aspects += fmt.Sprintf("- %s\n", a)
		}
	}

	return fmt.Sprintf(`请为以下主题搜集研究资料：

主题：%s

%s

请使用 web_search 工具搜索相关信息，并以 JSON 格式返回结果。`, input.Theme, aspects)
}

// parseVideoScript 解析视频脚本
func parseVideoScript(output string) *videoschema.VideoScript {
	// 尝试直接解析
	var script videoschema.VideoScript
	if err := json.Unmarshal([]byte(output), &script); err == nil {
		return &script
	}

	// 尝试提取 JSON 代码块
	script = videoschema.VideoScript{}
	if idx := indexOf(output, "```json"); idx >= 0 {
		start := idx + 7
		end := indexOf(output[start:], "```")
		if end >= 0 {
			jsonStr := output[start : start+end]
			if err := json.Unmarshal([]byte(jsonStr), &script); err == nil {
				return &script
			}
		}
	}

	// 尝试提取 ``` 代码块
	if idx := indexOf(output, "```"); idx >= 0 {
		start := idx + 3
		end := indexOf(output[start:], "```")
		if end >= 0 {
			jsonStr := output[start : start+end]
			if err := json.Unmarshal([]byte(jsonStr), &script); err == nil {
				return &script
			}
		}
	}

	// 如果都不是，创建一个只有一个镜头的脚本
	script = videoschema.VideoScript{
		Title: "视频脚本",
		Scenes: []videoschema.Scene{
			{
				Index:       1,
				Duration:    15,
				Description: "中景",
				Script:      output,
				Visual:      "待补充画面描述",
				CameraMove:  "固定",
			},
		},
	}
	return &script
}

// parseCriticOutput 解析评论家输出
func parseCriticOutput(output string) videoschema.CriticOutput {
	result := videoschema.CriticOutput{}

	// 尝试直接解析 JSON
	if err := json.Unmarshal([]byte(output), &result); err == nil {
		return result
	}

	// 尝试提取 JSON 代码块
	if idx := indexOf(output, "```json"); idx >= 0 {
		start := idx + 7
		end := indexOf(output[start:], "```")
		if end >= 0 {
			jsonStr := output[start : start+end]
			if err := json.Unmarshal([]byte(jsonStr), &result); err == nil {
				return result
			}
		}
	}

	// 尝试提取 approved
	if contains(output, "approved") || contains(output, "通过") || contains(output, "合格") {
		result.Approved = true
	}
	if contains(output, "不通过") || contains(output, "拒绝") || contains(output, "failed") {
		result.Approved = false
	}

	// 尝试提取评分
	result.Scores = videoschema.CriticScores{}
	// 简化处理：如果没有解析到，默认通过但需要修改
	result.Approved = false
	result.Feedback = output
	result.Issues = []string{"需要审查脚本内容"}

	return result
}

// indexOf 查找子字符串位置
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// contains 检查是否包含子字符串
func contains(s, substr string) bool {
	return indexOf(s, substr) >= 0
}

// ============================================================================
// 兼容性别名
// ============================================================================

// NewToolCallingManager NewManager 的别名
func NewToolCallingManager(cm model.ToolCallingChatModel) *Manager {
	return NewManager(cm)
}
