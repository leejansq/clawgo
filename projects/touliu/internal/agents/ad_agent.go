/*
 * Agent Base with LLM, Session, Memory, and Skills support
 * E-Commerce Advertising Multi-Agent System
 */

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/leejansq/clawgo/internal/memory"
	"github.com/leejansq/clawgo/internal/session"
	"github.com/leejansq/clawgo/internal/skill"
)

// AdAgent 广告 Agent 基础结构
type AdAgent struct {
	name              string
	description       string
	model             model.ChatModel
	toolModel         model.ToolCallingChatModel
	sessionStore      session.SessionStore
	memoryStore       memory.MemoryStore
	skillLoader       *skill.SkillLoader
	reactAgent        *react.Agent
	toolsConfig       *compose.ToolsNodeConfig
	systemPrompt      string
	knowledgeBaseDir  string
}

// AdAgentConfig Agent 配置
type AdAgentConfig struct {
	Name             string
	Description      string
	Model            model.ChatModel
	ToolCallingModel model.ToolCallingChatModel
	SessionCWD       string
	MemoryBaseDir    string
	KnowledgeBaseDir string // 专业知识库目录
	SkillSources     []skill.SkillSource
	SystemPrompt     string
}

// NewAdAgent 创建广告 Agent
func NewAdAgent(ctx context.Context, cfg *AdAgentConfig) (*AdAgent, error) {
	// 创建 Session Store
	sessionStore := session.NewSessionStore()
	if cfg.SessionCWD != "" {
		sessionStore.CreateSession(cfg.SessionCWD)
	}

	// 创建 Memory Store (自动检测并创建 embedder)
	embedder, _ := memory.DetectAndCreateEmbedder(ctx)
	memoryCfg := &memory.Config{
		BaseDir:  cfg.MemoryBaseDir,
		Embedder: embedder,
	}
	memStore, err := memory.New(ctx, memoryCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create memory store: %w", err)
	}

	// 创建 Skill Loader
	skillLoader := skill.NewSkillLoader(cfg.SkillSources)

	agent := &AdAgent{
		name:              cfg.Name,
		description:       cfg.Description,
		model:             cfg.Model,
		toolModel:         cfg.ToolCallingModel,
		sessionStore:      sessionStore,
		memoryStore:       memStore,
		skillLoader:       skillLoader,
		systemPrompt:      cfg.SystemPrompt,
		knowledgeBaseDir:  cfg.KnowledgeBaseDir,
	}

	// 初始化工具
	if err := agent.initTools(ctx); err != nil {
		return nil, fmt.Errorf("failed to init tools: %w", err)
	}

	return agent, nil
}

// initTools 初始化工具
func (a *AdAgent) initTools(ctx context.Context) error {
	// 获取技能
	entries, err := a.skillLoader.LoadAll()
	if err != nil {
		return err
	}

	// 过滤技能
	ctx2 := skill.DefaultFilterContext()
	filters := skill.DefaultFilters()
	filtered := skill.FilterSkills(entries, filters, ctx2)

	// 构建工具
	var tools []tool.BaseTool
	for _, entry := range filtered {
		if entry.Skill.DisableModelInvocation {
			continue
		}
		t := &skillToolAdapter{skill: entry.Skill}
		tools = append(tools, t)
	}

	// 添加内置工具
	tools = append(tools, a.getBuiltinTools()...)

	a.toolsConfig = &compose.ToolsNodeConfig{Tools: tools}

	// 创建 ReAct agent
	if a.toolModel != nil && len(tools) > 0 {
		a.reactAgent, err = react.NewAgent(ctx, &react.AgentConfig{
			ToolCallingModel: a.toolModel,
			ToolsConfig:      *a.toolsConfig,
			MaxStep:          10,
		})
	}

	return nil
}

// getBuiltinTools 获取内置工具
func (a *AdAgent) getBuiltinTools() []tool.BaseTool {
	return []tool.BaseTool{
		a.newMemoryReadTool(),
		a.newMemoryWriteTool(),
		a.newSessionAppendTool(),
	}
}

// skillToolAdapter 技能工具适配器
type skillToolAdapter struct {
	skill *skill.Skill
}

func (t *skillToolAdapter) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: t.skill.Name,
		Desc: t.skill.Description,
	}, nil
}

func (t *skillToolAdapter) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	return t.skill.Content, nil
}

// memoryReadTool 记忆读取工具
func (a *AdAgent) newMemoryReadTool() *memoryReadTool {
	return &memoryReadTool{agent: a}
}

type memoryReadTool struct {
	agent *AdAgent
}

func (t *memoryReadTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "memory_read",
		Desc: "Read from memory/knowledge base. Use this to search for past experiences, lessons learned, and historical data.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "Search query",
				Required: true,
			},
			"memoryType": {
				Type: schema.String,
				Desc: "Memory type: longterm or shortterm",
			},
			"limit": {
				Type: schema.Integer,
				Desc: "Maximum number of results",
			},
		}),
	}, nil
}

func (t *memoryReadTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var params struct {
		Query      string `json:"query"`
		MemoryType string `json:"memoryType"`
		Limit      int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", err
	}

	if params.Limit == 0 {
		params.Limit = 5
	}

	opts2 := []memory.SearchOption{memory.WithSearchLimit(params.Limit)}
	if params.MemoryType == "longterm" {
		opts2 = append(opts2, memory.WithSearchMemoryTypes(memory.MemoryTypeLongTerm))
	}

	results, err := t.agent.memoryStore.Search(ctx, params.Query, opts2...)
	if err != nil {
		return "", err
	}

	var output strings.Builder
	for _, r := range results {
		output.WriteString(fmt.Sprintf("[%s] %s\n", r.Snippet, r.Source))
	}
	return output.String(), nil
}

// memoryWriteTool 记忆写入工具
func (a *AdAgent) newMemoryWriteTool() *memoryWriteTool {
	return &memoryWriteTool{agent: a}
}

type memoryWriteTool struct {
	agent *AdAgent
}

func (t *memoryWriteTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "memory_write",
		Desc: "Write to memory/knowledge base. Use this to save important experiences, lessons learned, and campaign results.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"content": {
				Type:     schema.String,
				Desc:     "Content to write to memory",
				Required: true,
			},
			"memoryType": {
				Type: schema.String,
				Desc: "Memory type: longterm or shortterm",
			},
			"source": {
				Type: schema.String,
				Desc: "Source identifier",
			},
			"importance": {
				Type: schema.Integer,
				Desc: "Importance score (0-10)",
			},
			"tags": {
				Type: schema.String,
				Desc: "Tags (comma separated)",
			},
		}),
	}, nil
}

func (t *memoryWriteTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var params struct {
		Content    string `json:"content"`
		MemoryType string `json:"memoryType"`
		Source     string `json:"source"`
		Importance int    `json:"importance"`
		Tags       string `json:"tags"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", err
	}

	meta := memory.MemoryMeta{
		Source:     params.Source,
		Importance: params.Importance,
		Tags:       strings.Split(params.Tags, ","),
		CreatedAt:  time.Now(),
	}

	if params.MemoryType == "longterm" {
		meta.Type = memory.MemoryTypeLongTerm
	} else {
		meta.Type = memory.MemoryTypeShortTerm
		meta.Date = time.Now().Format("2006-01-02")
	}

	if err := t.agent.memoryStore.Write(ctx, params.Content, meta); err != nil {
		return "", err
	}
	return "Memory saved successfully", nil
}

// sessionAppendTool Session追加工具
func (a *AdAgent) newSessionAppendTool() *sessionAppendTool {
	return &sessionAppendTool{agent: a}
}

type sessionAppendTool struct {
	agent *AdAgent
}

func (t *sessionAppendTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "session_append",
		Desc: "Append a message to session context for tracking conversation history.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"role": {
				Type:     schema.String,
				Desc:     "Role: user or assistant",
				Required: true,
			},
			"content": {
				Type:     schema.String,
				Desc:     "Message content",
				Required: true,
			},
		}),
	}, nil
}

func (t *sessionAppendTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var params struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", err
	}

	_, err := t.agent.sessionStore.AppendMessage(params.Role, params.Content)
	if err != nil {
		return "", err
	}
	return "Message appended", nil
}

// Chat 使用 LLM 进行对话
func (a *AdAgent) Chat(ctx context.Context, userMsg string) (string, error) {
	// 构建完整的对话上下文
	messages := a.buildMessages(ctx, userMsg)

	// 打印 LLM 思考日志
	fmt.Printf("\n🤖 [LLM Input] 会话上下文 (%d 条消息)\n", len(messages))
	for i, msg := range messages {
		role := string(msg.Role)
		if role == "" {
			role = "unknown"
		}
		contentLen := len(msg.Content)
		if contentLen > 300 {
			fmt.Printf("   [%d] %s: %.300s...\n", i, role, msg.Content)
		} else {
			fmt.Printf("   [%d] %s: %s\n", i, role, msg.Content)
		}
	}
	fmt.Println()

	// 使用带重试的调用
	return a.callLLMWithRetry(ctx, messages)
}

// callLLMWithRetry 带重试的 LLM 调用
func (a *AdAgent) callLLMWithRetry(ctx context.Context, messages []*schema.Message) (string, error) {
	maxRetries := 3
	baseDelay := 2 * time.Second

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// 如果有 ReAct agent，使用工具调用
		if a.reactAgent != nil {
			result, err := a.reactAgent.Generate(ctx, messages)
			if err != nil {
				lastErr = err
				// 检查是否是可重试的错误（529 服务繁忙）
				if attempt < maxRetries && isRetryableError(err) {
					delay := baseDelay * time.Duration(attempt)
					fmt.Printf("⚠️  [LLM] 服务繁忙 (attempt %d/%d)，%v 后重试...\n", attempt, maxRetries, delay)
					time.Sleep(delay)
					continue
				}
				return "", err
			}
			fmt.Printf("🤖 [LLM Output]: %s%s\n\n", result.Content, func() string {
				if len(result.Content) > 300 {
					return "..."
				}
				return ""
			}())
			os.Stdout.Sync()
			return result.Content, nil
		}

		// 直接使用 ChatModel
		stream, err := a.model.Stream(ctx, messages)
		if err != nil {
			lastErr = err
			// 检查是否是可重试的错误
			if attempt < maxRetries && isRetryableError(err) {
				delay := baseDelay * time.Duration(attempt)
				fmt.Printf("⚠️  [LLM] 服务繁忙 (attempt %d/%d)，%v 后重试...\n", attempt, maxRetries, delay)
				time.Sleep(delay)
				continue
			}
			return "", err
		}
		defer stream.Close()

		var output strings.Builder
		for {
			frame, err := stream.Recv()
			if err != nil {
				break
			}
			if frame != nil && frame.Content != "" {
				output.WriteString(frame.Content)
			}
		}

		result := output.String()
		fmt.Printf("🤖 [LLM Output]: %.300s%s\n\n", result, func() string {
			if len(result) > 300 {
				return "..."
			}
			return ""
		}())
		os.Stdout.Sync()
		return result, nil
	}

	return "", lastErr
}

// isRetryableError 判断错误是否可重试
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// 529 服务繁忙、超时、临时性错误
	retryableErrors := []string{"529", "timeout", "temporary", "服务集群负载", "请稍后重试"}
	for _, keyword := range retryableErrors {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}
	return false
}

// buildMessages 从 session 构建完整的对话上下文
func (a *AdAgent) buildMessages(ctx context.Context, userMsg string) []*schema.Message {
	// 获取 session 历史
	branch := a.sessionStore.GetBranch()

	// 调试：打印 session 分支信息
	fmt.Printf("   [DEBUG] Session branch has %d entries\n", len(branch))
	for i, entry := range branch {
		if me, ok := entry.(*session.MessageEntry); ok {
			contentPreview := me.Message.Content
			if len(contentPreview) > 100 {
				contentPreview = contentPreview[:100] + "..."
			}
			fmt.Printf("   [DEBUG]   [%d] %s: %s\n", i, me.Message.Role, contentPreview)
		}
	}

	messages := []*schema.Message{}

	// 添加系统提示
	if a.systemPrompt != "" {
		messages = append(messages, schema.SystemMessage(a.systemPrompt))
	}

	// 添加历史消息
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

	// 添加当前用户消息
	messages = append(messages, schema.UserMessage(userMsg))

	return messages
}

// AppendSession 添加消息到会话
func (a *AdAgent) AppendSession(role, content string) error {
	_, err := a.sessionStore.AppendMessage(role, content)
	return err
}

// GetSessionBranch 获取会话分支
func (a *AdAgent) GetSessionBranch() []session.SessionEntry {
	return a.sessionStore.GetBranch()
}

// SearchMemory 搜索记忆
func (a *AdAgent) SearchMemory(ctx context.Context, query string, limit int) ([]*memory.SearchResult, error) {
	if limit == 0 {
		limit = 5
	}
	return a.memoryStore.Search(ctx, query, memory.WithSearchLimit(limit))
}

// WriteMemory 写入记忆
func (a *AdAgent) WriteMemory(ctx context.Context, content string, meta memory.MemoryMeta) error {
	return a.memoryStore.Write(ctx, content, meta)
}

// Close 关闭 Agent
func (a *AdAgent) Close() error {
	return a.memoryStore.Close()
}

// LoadKnowledgeFiles 加载知识库目录下的所有文件内容
// 支持 .txt, .md, .json 文件
func (a *AdAgent) LoadKnowledgeFiles() (string, error) {
	if a.knowledgeBaseDir == "" {
		return "", nil
	}

	var knowledge strings.Builder
	knowledge.WriteString("【专业知识库内容】\n\n")

	// 遍历目录读取文件
	err := filepath.Walk(a.knowledgeBaseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录
		if info.IsDir() {
			return nil
		}

		// 只处理文本文件
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".txt" && ext != ".md" && ext != ".json" {
			return nil
		}

		// 读取文件内容
		file, err := os.Open(path)
		if err != nil {
			fmt.Printf("⚠️  知识库文件读取失败: %s, err: %v\n", path, err)
			return nil
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			fmt.Printf("⚠️  知识库文件内容读取失败: %s, err: %v\n", path, err)
			return nil
		}

		knowledge.WriteString(fmt.Sprintf("【文件: %s】\n%s\n\n", filepath.Base(path), string(content)))
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("遍历知识库目录失败: %w", err)
	}

	return knowledge.String(), nil
}
