/*
 * Chat Bot Management System - ReAct Agent
 * 使用 eino ReAct agent 处理工具调用循环
 */

package chat

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/leejansq/clawgo/projects/chatbot/internal/manager"
)

// ToolProviderAdapter 适配 ToolProvider 到 eino tool.InvokableTool
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

// EinoToolAdapter 适配 ToolInfo 到 tool.InvokableTool
type EinoToolAdapter struct {
	info     *schema.ToolInfo
	handler  func(ctx context.Context, args map[string]any) (string, error)
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

// BuildToolsNodeConfig 从 ToolProvider 构建 compose.ToolsNodeConfig
func BuildToolsNodeConfig(providers []ToolProvider) *compose.ToolsNodeConfig {
	var tools []tool.BaseTool

	for _, p := range providers {
		for _, ti := range p.GetTools() {
			tools = append(tools, &ToolProviderAdapter{
				name:     ti.Name,
				provider: p,
			})
		}
	}

	return &compose.ToolsNodeConfig{
		Tools: tools,
	}
}

// BuildLocalToolsNodeConfig 从 manager 构建本地工具的 compose.ToolsNodeConfig
func BuildLocalToolsNodeConfig(mgr *manager.Manager) *compose.ToolsNodeConfig {
	localTools := BuildLocalTools(mgr)
	var tools []tool.BaseTool

	for _, ti := range localTools {
		info := &schema.ToolInfo{
			Name: ti.Name,
			Desc: ti.Description,
		}
		if ti.Params != nil {
			info.ParamsOneOf = ti.Params
		}
		tools = append(tools, &EinoToolAdapter{
			info:    info,
			handler: ti.Handler,
		})
	}

	return &compose.ToolsNodeConfig{
		Tools: tools,
	}
}

// NewReActAgent 创建 ReAct agent
func NewReActAgent(ctx context.Context, cm model.ToolCallingChatModel, toolsConfig *compose.ToolsNodeConfig) (*react.Agent, error) {
	config := &react.AgentConfig{
		ToolCallingModel: cm,
		ToolsConfig:      *toolsConfig,
		MaxStep:          10,
	}
	return react.NewAgent(ctx, config)
}
