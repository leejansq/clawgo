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
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/leejansq/clawgo/internal/session"
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
// Tool 适配器
// ============================================================================

// ToolProviderAdapter 适配 ToolProvider 到 eino tool.BaseTool
type ToolProviderAdapter struct {
	name     string
	provider ToolProvider
}

func (t *ToolProviderAdapter) Info(ctx context.Context) (*schema.ToolInfo, error) {
	tools := t.provider.GetTools()
	for _, ti := range tools {
		if ti.Name == t.name {
			return ti, nil
		}
	}
	return nil, fmt.Errorf("tool %s not found", t.name)
}

func (t *ToolProviderAdapter) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args map[string]any
	if argumentsInJSON != "" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
			return "", fmt.Errorf("failed to unmarshal arguments: %w", err)
		}
	}
	return t.provider.CallTool(ctx, t.name, args)
}

// EinoToolAdapter 适配自定义工具到 eino tool.BaseTool
type EinoToolAdapter struct {
	info    *schema.ToolInfo
	handler func(ctx context.Context, args map[string]any) (string, error)
}

func (t *EinoToolAdapter) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.info, nil
}

func (t *EinoToolAdapter) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args map[string]any
	if argumentsInJSON != "" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
			return "", fmt.Errorf("failed to unmarshal arguments: %w", err)
		}
	}
	return t.handler(ctx, args)
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
	mu                  sync.RWMutex
	sessions            map[string]*Session
	cm                  model.ToolCallingChatModel
	cmWithTools         model.ToolCallingChatModel // 缓存绑定工具后的模型
	reactAgent          *react.Agent               // 缓存的 React agent
	toolProviders       []ToolProvider
	conversationHistory []*schema.Message    // 多轮对话历史（按时间顺序存储）
	sessionStore        session.SessionStore // clawgo session store（用于分支）
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
	// 工具变更时清除缓存，下次会重新构建
	m.cmWithTools = nil
	m.reactAgent = nil
}

// ClearConversationHistory 清除对话历史（用于开始新的生成任务）
func (m *Manager) ClearConversationHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conversationHistory = nil
}

// SetSessionStore 设置 session store（用于分支机制）
func (m *Manager) SetSessionStore(s session.SessionStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionStore = s
}

// GetSessionStore 获取 session store
func (m *Manager) GetSessionStore() session.SessionStore {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessionStore
}

// AppendSessionMessage 追加消息到 session store（如果有）
func (m *Manager) AppendSessionMessage(role, content string) {
	m.mu.RLock()
	s := m.sessionStore
	m.mu.RUnlock()
	if s != nil {
		s.AppendMessage(role, content)
	}
}

// WithBypassedSession 临时跳过 session history，执行函数后恢复
// 用于 Critic 等不需要使用 session 分支的场景
func (m *Manager) WithBypassedSession(fn func() error) error {
	// 暂存当前 session store
	m.mu.Lock()
	origStore := m.sessionStore
	// 清空 session store
	m.sessionStore = nil
	m.mu.Unlock()

	// 执行函数
	err := fn()

	// 恢复 session store
	m.mu.Lock()
	m.sessionStore = origStore
	m.mu.Unlock()

	return err
}

// GetSessionBranch 获取当前 session branch
func (m *Manager) GetSessionBranch() []*schema.Message {
	m.mu.RLock()
	s := m.sessionStore
	m.mu.RUnlock()
	if s == nil {
		return nil
	}

	branch := s.GetBranch()
	if len(branch) == 0 {
		return nil
	}

	// 转换为 schema.Message
	var messages []*schema.Message
	for _, entry := range branch {
		if me, ok := entry.(*session.MessageEntry); ok {
			switch me.Message.Role {
			case "user":
				messages = append(messages, schema.UserMessage(me.Message.Content))
			case "assistant":
				messages = append(messages, &schema.Message{
					Role:    schema.RoleType("assistant"),
					Content: me.Message.Content,
				})
			default:
				messages = append(messages, schema.UserMessage(me.Message.Content))
			}
		}
	}
	return messages
}

// GetConversationHistory 获取对话历史
func (m *Manager) GetConversationHistory() []*schema.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.conversationHistory
}

// AppendToHistory 添加消息到对话历史
func (m *Manager) AppendToHistory(role schema.RoleType, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conversationHistory = append(m.conversationHistory, &schema.Message{
		Role:    role,
		Content: content,
	})
}

// GetAllTools 获取所有工具
func (m *Manager) GetAllTools() []*schema.ToolInfo {
	// m.mu.RLock()
	// defer m.mu.RUnlock()

	allTools := make([]*schema.ToolInfo, 0)
	for _, p := range m.toolProviders {
		allTools = append(allTools, p.GetTools()...)
	}
	return allTools
}

// getCmWithTools 获取绑定工具后的模型（带缓存）
func (m *Manager) getCmWithTools() (model.ToolCallingChatModel, error) {

	if m.cmWithTools != nil {
		return m.cmWithTools, nil
	}

	// 再次检查（可能有并发）
	if m.cmWithTools != nil {
		return m.cmWithTools, nil
	}

	allTools := m.GetAllTools()
	if len(allTools) == 0 {
		return m.cm, nil
	}

	cmWithTools, err := m.cm.WithTools(allTools)
	if err != nil {
		return nil, fmt.Errorf("failed to bind tools: %w", err)
	}
	m.cmWithTools = cmWithTools
	return m.cmWithTools, nil
}

// buildToolsNodeConfig 从 toolProviders 构建 compose.ToolsNodeConfig
func (m *Manager) buildToolsNodeConfig() (*compose.ToolsNodeConfig, error) {
	var tools []tool.BaseTool

	for _, p := range m.toolProviders {
		for _, ti := range p.GetTools() {
			tools = append(tools, &ToolProviderAdapter{
				name:     ti.Name,
				provider: p,
			})
		}
	}

	if len(tools) == 0 {
		return nil, fmt.Errorf("no tools available")
	}

	return &compose.ToolsNodeConfig{
		Tools: tools,
	}, nil
}

// getReactAgent 获取 React Agent（带缓存）
func (m *Manager) getReactAgent(ctx context.Context) (*react.Agent, error) {
	m.mu.RLock()
	if m.reactAgent != nil {
		m.mu.RUnlock()
		return m.reactAgent, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// 再次检查（可能有并发）
	if m.reactAgent != nil {
		return m.reactAgent, nil
	}

	// 获取绑定工具的模型
	cmWithTools, err := m.getCmWithTools()
	if err != nil {
		return nil, err
	}

	// 构建 tools config
	toolsConfig, err := m.buildToolsNodeConfig()
	if err != nil {
		return nil, err
	}

	// 创建 React agent
	config := &react.AgentConfig{
		ToolCallingModel: cmWithTools,
		ToolsConfig:      *toolsConfig,
		MaxStep:          10,
	}
	agent, err := react.NewAgent(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create React agent: %w", err)
	}

	m.reactAgent = agent
	return m.reactAgent, nil
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

// GenerateWithTools 调用 LLM 生成（使用 React Agent 处理工具调用循环）
func (m *Manager) GenerateWithTools(ctx context.Context, systemPrompt, userInput string) (string, error) {
	return m.GenerateWithToolsAndHistory(ctx, systemPrompt, userInput, nil)
}

// GenerateWithToolsAndHistory 调用 LLM 生成（支持多轮对话记忆）
func (m *Manager) GenerateWithToolsAndHistory(ctx context.Context, systemPrompt, userInput string, history []*schema.Message) (string, error) {
	// 如果有 session store，优先使用 session branch 作为历史
	branchHistory := m.GetSessionBranch()
	if len(branchHistory) > 0 {
		history = branchHistory
	}

	// 构建消息
	input := []*schema.Message{
		schema.SystemMessage(systemPrompt),
	}

	// 添加历史消息
	if history != nil {
		input = append(input, history...)
	}

	// 添加当前用户输入
	input = append(input, schema.UserMessage(userInput))

	// 打印 LLM 输入日志
	log.Printf("=== LLM INPUT ===")
	log.Printf("System: %s", systemPrompt)
	log.Printf("User: %s", userInput)
	if history != nil {
		log.Printf("History messages: %d", len(history))
	}
	log.Printf("=================")

	// 调用 React Agent（带 ARK bug 重试）
	result, err := m.callReactAgentWithRetry(ctx, input)
	if err != nil {
		return "", fmt.Errorf("LLM generation failed: %w", err)
	}

	log.Printf("=== LLM OUTPUT ===")
	log.Printf("Content: %s", result.Content)
	log.Printf("==================")

	return result.Content, nil
}

// callReactAgentWithRetry 调用 React Agent（带 ARK bug 重试）
func (m *Manager) callReactAgentWithRetry(ctx context.Context, input []*schema.Message) (*schema.Message, error) {
	// 获取 React Agent
	agent, err := m.getReactAgent(ctx)
	if err != nil {
		return nil, err
	}

	// 调用 agent.Generate（它内部处理工具调用循环）
	result, err := agent.Generate(ctx, input)
	if err != nil {
		// 检查是否是 ARK 模型工具调用 ID 问题
		errMsg := err.Error()
		if strings.Contains(errMsg, "tool id") && strings.Contains(errMsg, "not found") {
			// ARK 模型多轮工具调用 bug：清除缓存的 agent 并重试
			log.Printf("ARK model tool ID bug detected, clearing agent cache and retrying...")
			m.mu.Lock()
			m.reactAgent = nil
			m.mu.Unlock()

			// 重新获取 agent
			agent, err = m.getReactAgent(ctx)
			if err != nil {
				return nil, err
			}

			// 重试（只重试一次）
			result, err = agent.Generate(ctx, input)
			if err != nil {
				return nil, fmt.Errorf("ARK bug retry failed: %w", err)
			}
			log.Printf("ARK bug retry succeeded")
			return result, nil
		}
		return nil, err
	}

	return result, nil
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

	// 获取对话历史（用于多轮对话记忆）- 如果有 session store 会自动使用 branch
	history := m.GetConversationHistory()

	// 调用 LLM（带多轮对话历史）
	output, err := m.GenerateWithToolsAndHistory(ctx, session.SystemPrompt, userPrompt, history)
	if err != nil {
		m.UpdateSessionStatus(sessionKey, "error", "", err.Error())
		return nil, err
	}

	// 追加用户消息和助手回复到 session store（如果存在）
	m.AppendSessionMessage("user", userPrompt)
	m.AppendSessionMessage("assistant", output)

	// 同时保留一份到内存历史（兼容）
	m.AppendToHistory(schema.RoleType("user"), userPrompt)
	m.AppendToHistory(schema.RoleType("assistant"), output)

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
