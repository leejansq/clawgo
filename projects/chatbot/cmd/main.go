/*
 * Chat Bot Management System - Main Entry Point
 * 启动主 Agent 管理器和服务，支持 MCP
 */

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/leejansq/clawgo/projects/chatbot/internal/chat"
	"github.com/leejansq/clawgo/projects/chatbot/internal/manager"
	"github.com/leejansq/clawgo/projects/chatbot/internal/mcp"
	"github.com/leejansq/clawgo/projects/chatbot/internal/server"
	"github.com/leejansq/clawgo/projects/chatbot/internal/store"
)

func main() {
	// 命令行参数
	addr := flag.String("addr", "127.0.0.1:18888", "Server address")
	dbPath := flag.String("db", "./chatbot.db", "SQLite database path")
	staticDir := flag.String("static", "", "Static files directory")
	mcpServer := flag.Bool("mcp-server", false, "Enable MCP server mode (stdio transport)")
	mcpClient := flag.String("mcp-client", "", "MCP client to connect (format: name=xxx,cmd='command args')")
	flag.Parse()

	// 确保数据库目录存在
	dbDir := filepath.Dir(*dbPath)
	if dbDir != "." && dbDir != "" {
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			log.Fatalf("Failed to create database directory: %v", err)
		}
	}

	// 初始化 SQLite 存储
	dbStore, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer dbStore.Close()

	log.Printf("SQLite database: %s", *dbPath)

	// 初始化管理器
	mgr := manager.NewManager(dbStore)

	// MCP 服务器模式
	if *mcpServer {
		startMCPServer(mgr)
		return
	}

	// 初始化 LLM 模型
	cm, err := newChatModel(context.Background())
	if err != nil {
		log.Fatalf("Failed to newChatModel: %v", err)
	}
	adapter := chat.NewEinoChatModelAdapter(cm)

	// 注册本地工具
	localTools := chat.BuildLocalTools(mgr)
	registry := chat.NewToolRegistry()
	for _, t := range localTools {
		registry.Register(t)
	}
	adapter.RegisterToolProvider(&localToolProvider{registry: registry})

	// 连接外部 MCP 客户端
	if *mcpClient != "" {
		connectMCPClients(context.Background(), adapter, *mcpClient)
	}

	chatHandler := chat.NewChatHandler(mgr, adapter)

	// 初始化服务器
	srv := server.NewServer(mgr, chatHandler, *staticDir)

	// 启动服务器
	log.Println("Chat Bot Management System")
	log.Println("================================")
	log.Printf("HTTP API: http://%s", *addr)
	log.Printf("Web Console: http://%s/chat/{{sessionKey}}", *addr)
	if *mcpClient != "" {
		log.Printf("MCP Client: %s", *mcpClient)
	}
	log.Println("================================")

	if err := srv.Start(*addr); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// localToolProvider 本地工具提供者
type localToolProvider struct {
	registry *chat.ToolRegistry
}

func (p *localToolProvider) GetTools() []*schema.ToolInfo {
	return p.registry.GetTools()
}

func (p *localToolProvider) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	return p.registry.CallTool(ctx, name, args)
}

func newChatModel(ctx context.Context) (model.ToolCallingChatModel, error) {
	if os.Getenv("MODEL_TYPE") == "ark" {
		return ark.NewChatModel(ctx, &ark.ChatModelConfig{
			APIKey:  os.Getenv("ARK_API_KEY"),
			Model:   os.Getenv("ARK_MODEL"),
			BaseURL: os.Getenv("ARK_BASE_URL"),
		})
	}
	return openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		Model:   os.Getenv("OPENAI_MODEL"),
		BaseURL: os.Getenv("OPENAI_BASE_URL"),
		ByAzure: os.Getenv("OPENAI_BY_AZURE") == "true",
	})
}

// startMCPServer 启动 MCP 服务器
func startMCPServer(mgr *manager.Manager) {
	srv := mcp.NewMCPServer("chatbot", "1.0.0")

	// 注册会话管理工具
	registerMCP_tools(srv, mgr)

	log.Println("Starting MCP server on stdio...")
	if err := srv.Start(context.Background()); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}

// registerMCP_tools 注册 MCP 工具到服务器
func registerMCP_tools(srv *mcp.MCPServer, mgr *manager.Manager) {
	// create_session
	srv.RegisterTool(mcp.Tool{
		Name:        "create_session",
		Description: "Create a new chat session",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"label":        map[string]any{"type": "string", "description": "Session label"},
				"systemPrompt": map[string]any{"type": "string", "description": "System prompt"},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		var req chat.CreateSessionRequest
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		session, sessionKey, err := mgr.CreateSession(req.Label, req.SystemPrompt)
		if err != nil {
			return nil, err
		}
		resp := chat.CreateSessionResponse{
			SessionKey: sessionKey,
			Label:      session.Label,
			Status:     session.Status,
		}
		return json.Marshal(resp)
	})

	// get_session
	srv.RegisterTool(mcp.Tool{
		Name:        "get_session",
		Description: "Get session details",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sessionKey": map[string]any{"type": "string", "description": "Session key"},
			},
			"required": []string{"sessionKey"},
		},
	}, func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		var req chat.GetSessionRequest
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		session, err := mgr.GetSession(req.SessionKey)
		if err != nil || session == nil {
			return nil, fmt.Errorf("session not found: %s", req.SessionKey)
		}
		resp := chat.GetSessionResponse{
			SessionKey:   session.SessionKey,
			Label:        session.Label,
			SystemPrompt: session.SystemPrompt,
			Status:       session.Status,
		}
		for _, msg := range session.GetMessages() {
			resp.Messages = append(resp.Messages, struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}{Role: msg.Role, Content: msg.Content})
		}
		return json.Marshal(resp)
	})

	// list_sessions
	srv.RegisterTool(mcp.Tool{
		Name:        "list_sessions",
		Description: "List all sessions",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		sessions, err := mgr.ListSessions()
		if err != nil {
			return nil, err
		}
		resp := chat.ListSessionsResponse{
			Sessions: make([]struct {
				SessionKey string `json:"sessionKey"`
				Label      string `json:"label"`
				Status     string `json:"status"`
			}, 0, len(sessions)),
		}
		for _, s := range sessions {
			resp.Sessions = append(resp.Sessions, struct {
				SessionKey string `json:"sessionKey"`
				Label      string `json:"label"`
				Status     string `json:"status"`
			}{SessionKey: s.SessionKey, Label: s.Label, Status: s.Status})
		}
		return json.Marshal(resp)
	})

	// delete_session
	srv.RegisterTool(mcp.Tool{
		Name:        "delete_session",
		Description: "Delete a session",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sessionKey": map[string]any{"type": "string", "description": "Session key"},
			},
			"required": []string{"sessionKey"},
		},
	}, func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		var req chat.DeleteSessionRequest
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		if err := mgr.DeleteSession(req.SessionKey); err != nil {
			return nil, err
		}
		return json.Marshal(chat.DeleteSessionResponse{Status: "deleted"})
	})

	// add_message
	srv.RegisterTool(mcp.Tool{
		Name:        "add_message",
		Description: "Add a message to a session",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sessionKey": map[string]any{"type": "string", "description": "Session key"},
				"role":       map[string]any{"type": "string", "description": "Message role (user/assistant)"},
				"content":    map[string]any{"type": "string", "description": "Message content"},
			},
			"required": []string{"sessionKey", "role", "content"},
		},
	}, func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		var req chat.AddMessageRequest
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		msg, err := mgr.AddMessage(req.SessionKey, req.Role, req.Content)
		if err != nil {
			return nil, err
		}
		return json.Marshal(chat.AddMessageResponse{ID: msg.ID, Status: "added"})
	})
}

// parseClientSpec 解析客户端规范字符串
// 格式: name=xxx cmd='command args' env='KEY1=val1,KEY2=val2'
func parseClientSpec(spec string) map[string]string {
	result := make(map[string]string)

	// 用空格分割，但保留引号内的空格
	fields := strings.Split(spec, " ")

	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}

		// 查找第一个 = 的位置
		idx := strings.Index(field, "=")
		if idx == -1 {
			continue
		}

		key := strings.TrimSpace(field[:idx])
		value := strings.TrimSpace(field[idx+1:])
		// 去除首尾引号
		value = strings.Trim(value, "\"'")

		if key != "" {
			result[key] = value
		}
	}

	return result
}

// splitWithQuote 用空格分割字符串，但保留引号内的空格
func splitWithQuote(s string) []string {
	var result []string
	var current string
	var inQuote bool
	var quoteChar rune

	for _, c := range s {
		if c == '"' || c == '\'' {
			if !inQuote {
				inQuote = true
				quoteChar = c
			} else if c == quoteChar {
				inQuote = false
			} else {
				current += string(c)
			}
		} else if c == ' ' && !inQuote {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}

	if current != "" {
		result = append(result, current)
	}

	return result
}

// unquote 去除字符串首尾的引号
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// connectMCPClients 连接外部 MCP 客户端
func connectMCPClients(ctx context.Context, adapter *chat.EinoChatModelAdapter, mcpClientSpec string) {
	// 解析格式: name=xxx,cmd='command args',env='KEY1=val1,KEY2=val2'
	// 多个客户端用分号分隔

	clients := strings.Split(mcpClientSpec, ";")
	for _, clientSpec := range clients {
		//clientSpec = strings.TrimSpace(clientSpec)
		if clientSpec == "" {
			continue
		}

		var name, cmd string
		var envList []string

		// 解析键值对
		parts := parseClientSpec(clientSpec)
		for k, v := range parts {
			switch k {
			case "name":
				name = v
			case "cmd":
				cmd = unquote(v)
			case "env":
				// 解析环境变量，格式: KEY1=val1,KEY2=val2
				envStr := unquote(v)
				envPairs := strings.Split(envStr, ",")
				for _, pair := range envPairs {
					pair = strings.TrimSpace(pair)
					if pair != "" {
						envList = append(envList, pair)
					}
				}
			}
		}
		log.Print(name, cmd, envList)

		if name == "" || cmd == "" {
			log.Printf("Invalid MCP client spec: %s (expected format: name=xxx cmd='command' env='KEY=val')", clientSpec)
			continue
		}

		log.Printf("Connecting to MCP server: %s (cmd: %s, env: %v)", name, cmd, envList)

		mcpClient := mcp.NewMCPClient(name, cmd, nil, envList)
		if err := mcpClient.Connect(ctx); err != nil {
			log.Printf("Failed to connect to MCP client %s: %v", name, err)
			continue
		}
		defer mcpClient.Close()

		// 注册 MCP 工具到适配器
		adapter.RegisterToolProvider(chat.NewMCPToolProvider(mcpClient))
		log.Printf("Connected to MCP server: %s", name)
	}
}
