/*
 * Chat Bot Management System - Tools
 * 本地工具处理器实现
 */

package chat

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/schema"
	"github.com/leejansq/clawgo/projects/chatbot/internal/manager"
)

// ToolHandler 本地工具处理函数
type ToolHandler func(ctx context.Context, args map[string]any) (string, error)

// ToolInfo 工具信息
type ToolInfo struct {
	Name        string
	Description string
	Handler     ToolHandler
	Params      *schema.ParamsOneOf
}

// ToolRegistry 工具注册表
type ToolRegistry struct {
	tools map[string]ToolInfo
}

// NewToolRegistry 创建工具注册表
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]ToolInfo),
	}
}

// Register 注册工具
func (r *ToolRegistry) Register(info ToolInfo) {
	r.tools[info.Name] = info
}

// GetTools 获取所有工具的 schema 信息
func (r *ToolRegistry) GetTools() []*schema.ToolInfo {
	result := make([]*schema.ToolInfo, 0, len(r.tools))
	for _, t := range r.tools {
		ti := &schema.ToolInfo{
			Name: t.Name,
			Desc: t.Description,
		}
		if t.Params != nil {
			ti.ParamsOneOf = t.Params
		}
		result = append(result, ti)
	}
	return result
}

// CallTool 调用工具
func (r *ToolRegistry) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("tool %s not found", name)
	}
	return t.Handler(ctx, args)
}

// CreateSessionRequest 创建会话请求
type CreateSessionRequest struct {
	Label        string `json:"label"`
	SystemPrompt string `json:"systemPrompt"`
}

// CreateSessionResponse 创建会话响应
type CreateSessionResponse struct {
	SessionKey string `json:"sessionKey"`
	Label      string `json:"label"`
	Status     string `json:"status"`
}

// GetSessionRequest 获取会话请求
type GetSessionRequest struct {
	SessionKey string `json:"sessionKey"`
}

// GetSessionResponse 获取会话响应
type GetSessionResponse struct {
	SessionKey   string `json:"sessionKey"`
	Label        string `json:"label"`
	SystemPrompt string `json:"systemPrompt"`
	Status       string `json:"status"`
	Messages     []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

// ListSessionsResponse 列出会话响应
type ListSessionsResponse struct {
	Sessions []struct {
		SessionKey string `json:"sessionKey"`
		Label      string `json:"label"`
		Status     string `json:"status"`
	} `json:"sessions"`
}

// DeleteSessionRequest 删除会话请求
type DeleteSessionRequest struct {
	SessionKey string `json:"sessionKey"`
}

// DeleteSessionResponse 删除会话响应
type DeleteSessionResponse struct {
	Status string `json:"status"`
}

// AddMessageRequest 添加消息请求
type AddMessageRequest struct {
	SessionKey string `json:"sessionKey"`
	Role       string `json:"role"`
	Content    string `json:"content"`
}

// AddMessageResponse 添加消息响应
type AddMessageResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

// CreateSessionToolHandler 创建会话工具处理器
func CreateSessionToolHandler(mgr *manager.Manager) ToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		req := CreateSessionRequest{}
		data, err := json.Marshal(args)
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(data, &req); err != nil {
			return "", err
		}

		session, sessionKey, err := mgr.CreateSession(req.Label, req.SystemPrompt)
		if err != nil {
			return "", err
		}

		resp := CreateSessionResponse{
			SessionKey: sessionKey,
			Label:      session.Label,
			Status:     session.Status,
		}
		result, _ := json.Marshal(resp)
		return string(result), nil
	}
}

// GetSessionToolHandler 获取会话工具处理器
func GetSessionToolHandler(mgr *manager.Manager) ToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		req := GetSessionRequest{}
		data, err := json.Marshal(args)
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(data, &req); err != nil {
			return "", err
		}

		session, err := mgr.GetSession(req.SessionKey)
		if err != nil || session == nil {
			return "", fmt.Errorf("session not found: %s", req.SessionKey)
		}

		resp := GetSessionResponse{
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

		result, _ := json.Marshal(resp)
		return string(result), nil
	}
}

// ListSessionsToolHandler 列出会话工具处理器
func ListSessionsToolHandler(mgr *manager.Manager) ToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		sessions, err := mgr.ListSessions()
		if err != nil {
			return "", err
		}

		resp := ListSessionsResponse{
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

		result, _ := json.Marshal(resp)
		return string(result), nil
	}
}

// DeleteSessionToolHandler 删除会话工具处理器
func DeleteSessionToolHandler(mgr *manager.Manager) ToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		req := DeleteSessionRequest{}
		data, err := json.Marshal(args)
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(data, &req); err != nil {
			return "", err
		}

		if err := mgr.DeleteSession(req.SessionKey); err != nil {
			return "", err
		}

		resp := DeleteSessionResponse{Status: "deleted"}
		result, _ := json.Marshal(resp)
		return string(result), nil
	}
}

// AddMessageToolHandler 添加消息工具处理器
func AddMessageToolHandler(mgr *manager.Manager) ToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		req := AddMessageRequest{}
		data, err := json.Marshal(args)
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(data, &req); err != nil {
			return "", err
		}

		msg, err := mgr.AddMessage(req.SessionKey, req.Role, req.Content)
		if err != nil {
			return "", err
		}

		resp := AddMessageResponse{ID: msg.ID, Status: "added"}
		result, _ := json.Marshal(resp)
		return string(result), nil
	}
}

// BuildLocalTools 创建本地工具列表
func BuildLocalTools(mgr *manager.Manager) []ToolInfo {
	return []ToolInfo{
		{
			Name:        "create_session",
			Description: "Create a new chat session",
			Handler:     CreateSessionToolHandler(mgr),
			Params: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"label":        {Type: schema.String, Desc: "Session label"},
				"systemPrompt": {Type: schema.String, Desc: "System prompt"},
			}),
		},
		{
			Name:        "get_session",
			Description: "Get session details by session key",
			Handler:     GetSessionToolHandler(mgr),
			Params: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"sessionKey": {Type: schema.String, Desc: "Session key"},
			}),
		},
		{
			Name:        "list_sessions",
			Description: "List all chat sessions",
			Handler:     ListSessionsToolHandler(mgr),
			Params:      nil,
		},
		{
			Name:        "delete_session",
			Description: "Delete a chat session",
			Handler:     DeleteSessionToolHandler(mgr),
			Params: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"sessionKey": {Type: schema.String, Desc: "Session key"},
			}),
		},
		{
			Name:        "add_message",
			Description: "Add a message to a session",
			Handler:     AddMessageToolHandler(mgr),
			Params: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"sessionKey": {Type: schema.String, Desc: "Session key"},
				"role":       {Type: schema.String, Desc: "Message role (user/assistant)"},
				"content":    {Type: schema.String, Desc: "Message content"},
			}),
		},
	}
}
