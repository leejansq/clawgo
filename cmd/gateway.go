package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// GatewayConfig Gateway 配置
type GatewayConfig struct {
	Port     int    // 端口，默认 18789
	Bind     string // 绑定地址，127.0.0.1 或 0.0.0.0
	Token    string // 认证 Token
	Password string // 认证密码
}

// AuthMode 认证模式
type AuthMode string

const (
	AuthModeToken    AuthMode = "token"
	AuthModePassword AuthMode = "password"
	AuthModeNone     AuthMode = "none"
)

// Gateway OpenClaw Gateway 服务器
type GatewayServer struct {
	config     *GatewayConfig
	authMode   AuthMode
	manager    *SubAgentManager
	channelMgr *ChannelManager
	sseClients map[string]map[string]*SSEClient
	clientsMu  sync.RWMutex
	httpServer *http.Server
}

// SSEClient SSE 客户端
type SSEClient struct {
	ID        string
	SessionKey string
	Writer    http.ResponseWriter
	Flusher   http.Flusher
	Done      chan struct{}
}

// ChatCompletionRequest OpenAI Chat Completion 请求
type ChatCompletionRequest struct {
	Model    string                   `json:"model"`
	Messages []map[string]interface{} `json:"messages"`
	Stream   bool                     `json:"stream,omitempty"`
}

// ChatCompletionResponse OpenAI Chat Completion 响应
type ChatCompletionResponse struct {
	ID      string      `json:"id"`
	Object  string      `json:"object"`
	Created int64       `json:"created"`
	Model   string      `json:"model"`
	Choices []Choice    `json:"choices"`
	Usage   Usage       `json:"usage"`
}

// Choice 选择
type Choice struct {
	Index        int         `json:"index"`
	Message      interface{} `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// Usage 使用量
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// NewGateway 创建 Gateway 实例
func NewGateway(config *GatewayConfig, manager *SubAgentManager) *GatewayServer {
	authMode := AuthModeNone
	if config.Token != "" {
		authMode = AuthModeToken
	} else if config.Password != "" {
		authMode = AuthModePassword
	}

	// 创建渠道管理器
	channelMgr := NewChannelManager()

	return &GatewayServer{
		config:     config,
		authMode:   authMode,
		manager:    manager,
		channelMgr: channelMgr,
		sseClients: make(map[string]map[string]*SSEClient),
	}
}

// generateGWKey 生成随机会话 key
func generateGWKey() string {
	return fmt.Sprintf("gw-%d", time.Now().UnixNano())
}

// Start 启动 Gateway 服务器
func (g *GatewayServer) Start(ctx context.Context) error {
	port := g.config.Port
	if port == 0 {
		port = 18789
	}

	bind := g.config.Bind
	if bind == "" {
		bind = "127.0.0.1"
	}

	addr := fmt.Sprintf("%s:%d", bind, port)

	// 创建 mux
	mux := http.NewServeMux()

	// 路由
	mux.HandleFunc("/health", g.handleHealth)
	mux.HandleFunc("/v1/models", g.handleListModels)
	mux.HandleFunc("/v1/chat/stream", g.handleChatStream)
	mux.HandleFunc("/v1/chat/completions", g.handleChatCompletions)
	mux.HandleFunc("/ws", g.handleWebSocket)
	mux.HandleFunc("/api/sessions", g.handleListSessions)
	mux.HandleFunc("/api/session/", g.handleGetSession)

	// Channel 路由
	mux.HandleFunc("/api/channels", g.handleListChannels)
	mux.HandleFunc("/api/channel/", g.handleChannelAction)
	mux.HandleFunc("/api/channel/webhook/", g.handleChannelWebhook)

	// 认证包装
	var handler http.Handler
	if g.authMode == AuthModeNone {
		handler = mux
	} else {
		handler = g.authMiddleware(mux)
	}

	g.httpServer = &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	fmt.Printf("🚀 [Gateway] Starting on http://%s\n", addr)
	fmt.Printf("🔐 [Gateway] Auth mode: %s\n", g.authMode)

	go func() {
		if err := g.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("❌ [Gateway] Server error: %v\n", err)
		}
	}()

	return nil
}

// authMiddleware 认证中间件
func (g *GatewayServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 跳过健康检查
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		var authenticated bool

		if g.authMode == AuthModeToken {
			// Token 认证: 检查 Authorization header
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				token := strings.TrimPrefix(auth, "Bearer ")
				if token == g.config.Token {
					authenticated = true
				}
			}
		} else if g.authMode == AuthModePassword {
			// Password 认证: 检查 X-Password header
			password := r.Header.Get("X-Password")
			if password == g.config.Password {
				authenticated = true
			}
		}

		if !authenticated {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Unauthorized",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleHealth 健康检查
func (g *GatewayServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// handleListModels 列出可用模型
func (g *GatewayServer) handleListModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "list",
		"data": []map[string]string{
			{
				"id":       "gpt-4",
				"object":   "model",
				"created":  "2023-03-12",
				"owned_by": "openai",
			},
			{
				"id":       "gpt-3.5-turbo",
				"object":   "model",
				"created":  "2023-03-12",
				"owned_by": "openai",
			},
		},
	})
}

// handleChatCompletions 处理 Chat Completions 请求 (非流式)
func (g *GatewayServer) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 提取最后一条用户消息
	var lastUserMessage string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if role, ok := msg["role"].(string); ok && role == "user" {
			if content, ok := msg["content"].(string); ok {
				lastUserMessage = content
				break
			}
		}
	}

	if lastUserMessage == "" {
		http.Error(w, "No user message found", http.StatusBadRequest)
		return
	}

	// 使用现有工作区或创建新的
	workspace := g.manager.GetExistingWorkspace()
	if workspace == "" {
		workspace = "./workspace"
	}

	config := &SubAgentConfig{
		Label:   "gateway-task",
		Task:    lastUserMessage,
		Timeout: 5 * time.Minute,
		Cleanup: "keep",
	}

	// 异步执行任务
	info := g.manager.SpawnAsyncInWorkspace(r.Context(), config, workspace)

	// 等待结果 (简化处理: 直接轮询等待)
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		result := g.manager.GetResult(info.SessionKey)
		if result != nil && (result.Status == "completed" || result.Status == "error") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(ChatCompletionResponse{
				ID:      "chatcmpl-" + generateGWKey(),
				Object:  "chat.completion",
				Created: time.Now().Unix(),
				Model:   req.Model,
				Choices: []Choice{
					{
						Index: 0,
						Message: map[string]string{
							"role":    "assistant",
							"content": result.Result,
						},
						FinishReason: "stop",
					},
				},
				Usage: Usage{
					PromptTokens:     0,
					CompletionTokens: 0,
					TotalTokens:      0,
				},
			})
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	http.Error(w, "Task timeout", http.StatusGatewayTimeout)
}

// handleChatStream 处理 SSE 流式聊天
func (g *GatewayServer) handleChatStream(w http.ResponseWriter, r *http.Request) {
	sessionKey := r.URL.Query().Get("sessionKey")
	clientID := generateGWKey()

	// 设置 SSE 头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// 注册客户端
	client := &SSEClient{
		ID:         clientID,
		SessionKey: sessionKey,
		Writer:     w,
		Flusher:    flusher,
		Done:      make(chan struct{}),
	}
	g.registerClient(sessionKey, clientID, client)

	// 发送初始消息
	fmt.Fprintf(w, "data: %s\n\n", `{"type":"connected","id":"`+clientID+`"}`)
	flusher.Flush()

	// 监听 Announce 事件
	go func() {
		for {
			select {
			case <-client.Done:
				return
			case announce := <-g.manager.announceChan:
				if sessionKey == "" || announce.SessionKey == sessionKey {
					data, _ := json.Marshal(map[string]interface{}{
						"type":   "message",
						"content": announce.Result,
						"status": announce.Status,
					})
					fmt.Fprintf(w, "data: %s\n\n", data)
					flusher.Flush()

					if announce.Status == "completed" || announce.Status == "error" {
						return
					}
				}
			}
		}
	}()

	// 保持连接
	<-r.Context().Done()
	g.unregisterClient(sessionKey, clientID)
}

// handleWebSocket 处理 WebSocket 连接 (简化版，使用 SSE)
func (g *GatewayServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionKey := r.URL.Query().Get("sessionKey")
	if sessionKey == "" {
		sessionKey = "default"
	}

	clientID := generateGWKey()

	// 设置 SSE 头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// 注册客户端
	client := &SSEClient{
		ID:         clientID,
		SessionKey: sessionKey,
		Writer:     w,
		Flusher:    flusher,
		Done:      make(chan struct{}),
	}
	g.registerClient(sessionKey, clientID, client)

	// 发送连接成功消息
	fmt.Fprintf(w, "data: %s\n\n", `{"type":"connected","id":"`+clientID+`"}`)
	flusher.Flush()

	// 监听 Announce 事件
	for {
		select {
		case <-r.Context().Done():
			g.unregisterClient(sessionKey, clientID)
			return
		case announce := <-g.manager.announceChan:
			if sessionKey == "" || announce.SessionKey == sessionKey {
				data, _ := json.Marshal(map[string]interface{}{
					"type":   "message",
					"content": announce.Result,
					"status": announce.Status,
				})
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()

				if announce.Status == "completed" || announce.Status == "error" {
					break
				}
			}
		}
	}
}

// handleListSessions 列出所有会话
func (g *GatewayServer) handleListSessions(w http.ResponseWriter, r *http.Request) {
	// 获取会话列表
	sessions := g.manager.ListActiveSessions()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sessions": sessions,
		"total":    len(sessions),
	})
}

// handleGetSession 获取指定会话
func (g *GatewayServer) handleGetSession(w http.ResponseWriter, r *http.Request) {
	// 提取 sessionKey
	sessionKey := strings.TrimPrefix(r.URL.Path, "/api/session/")
	if sessionKey == "" {
		http.Error(w, "Session key required", http.StatusBadRequest)
		return
	}

	info := g.manager.GetResult(sessionKey)
	if info == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// registerClient 注册 SSE 客户端
func (g *GatewayServer) registerClient(sessionKey, clientID string, client *SSEClient) {
	g.clientsMu.Lock()
	defer g.clientsMu.Unlock()

	if g.sseClients[sessionKey] == nil {
		g.sseClients[sessionKey] = make(map[string]*SSEClient)
	}
	g.sseClients[sessionKey][clientID] = client
}

// unregisterClient 注销 SSE 客户端
func (g *GatewayServer) unregisterClient(sessionKey, clientID string) {
	g.clientsMu.Lock()
	defer g.clientsMu.Unlock()

	if g.sseClients[sessionKey] != nil {
		delete(g.sseClients, clientID)
		if len(g.sseClients[sessionKey]) == 0 {
			delete(g.sseClients, sessionKey)
		}
	}
}

// ============================================================================
// Channel 处理器
// ============================================================================

// handleListChannels 列出所有渠道
func (g *GatewayServer) handleListChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// 列出所有渠道
		channels := g.channelMgr.List()
		result := make([]map[string]interface{}, 0, len(channels))
		for _, ch := range channels {
			result = append(result, map[string]interface{}{
				"id":   ch.GetID(),
				"type": ch.GetType(),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"channels": result,
			"total":    len(result),
		})

	case "POST":
		// 创建新渠道
		var config ChannelConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// 创建渠道
		var channel Channel
		switch config.Type {
		case ChannelTypeWebhook:
			if config.Webhook == nil {
				http.Error(w, "Webhook config required", http.StatusBadRequest)
				return
			}
			channel = NewWebhookChannel(config.Webhook)
		default:
			http.Error(w, "Unsupported channel type", http.StatusBadRequest)
			return
		}

		// 注册渠道
		g.channelMgr.Register(channel)

		// 启动渠道
		if err := channel.Start(r.Context()); err != nil {
			http.Error(w, fmt.Sprintf("Failed to start channel: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "ok",
			"channel": map[string]string{
				"id":   channel.GetID(),
				"type": string(channel.GetType()),
			},
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleChannelAction 渠道操作
func (g *GatewayServer) handleChannelAction(w http.ResponseWriter, r *http.Request) {
	// 提取 channel ID
	channelID := strings.TrimPrefix(r.URL.Path, "/api/channel/")
	if channelID == "" {
		http.Error(w, "Channel ID required", http.StatusBadRequest)
		return
	}

	channel := g.channelMgr.Get(channelID)
	if channel == nil {
		http.Error(w, "Channel not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case "GET":
		// 获取渠道信息
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":   channel.GetID(),
			"type": channel.GetType(),
		})

	case "DELETE":
		// 删除渠道
		channel.Stop()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "deleted",
		})

	case "POST":
		// 发送消息
		var req struct {
			To      string `json:"to"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if err := channel.SendMessage(req.To, req.Message); err != nil {
			http.Error(w, fmt.Sprintf("Failed to send message: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "sent",
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleChannelWebhook 处理渠道 Webhook 回调
func (g *GatewayServer) handleChannelWebhook(w http.ResponseWriter, r *http.Request) {
	// 提取 channel ID
	channelID := strings.TrimPrefix(r.URL.Path, "/api/channel/webhook/")
	if channelID == "" {
		http.Error(w, "Channel ID required", http.StatusBadRequest)
		return
	}

	channel := g.channelMgr.Get(channelID)
	if channel == nil {
		http.Error(w, "Channel not found", http.StatusNotFound)
		return
	}

	// 委托给渠道处理
	channel.HandleWebhook(w, r)
}
