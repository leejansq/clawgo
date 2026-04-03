/*
 * Video Script Generation - Tool Provider
 * 工具提供者，用于将 MCP 或其他工具注册到 LLM
 */

package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
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
// MCP 工具提供者
// ============================================================================

// MCPToolProvider MCP 工具提供者
type MCPToolProvider struct {
	client *MCPClient
}

// NewMCPToolProvider 创建 MCP 工具提供者
func NewMCPToolProvider(client *MCPClient) *MCPToolProvider {
	return &MCPToolProvider{client: client}
}

// GetTools 获取 MCP 工具列表
func (p *MCPToolProvider) GetTools() []*schema.ToolInfo {
	tools := p.client.ListTools()
	result := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
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

// ============================================================================
// 本地 WebSearch 工具提供者
// ============================================================================

// WebSearchToolProvider 本地 web_search 工具提供者
type WebSearchToolProvider struct {
	tool *WebSearchTool
}

// NewWebSearchToolProvider 创建 web_search 工具提供者
func NewWebSearchToolProvider() *WebSearchToolProvider {
	return &WebSearchToolProvider{
		tool: NewWebSearchTool(),
	}
}

// GetTools 获取工具列表
func (p *WebSearchToolProvider) GetTools() []*schema.ToolInfo {
	return []*schema.ToolInfo{
		{
			Name: "web_search",
			Desc: "Search the internet for information, news, and data. Use this to find relevant facts, statistics, case studies, and latest developments for your video script.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"query": {
					Type:        schema.String,
					Desc:        "The search query",
					Required:    true,
				},
				"max_results": {
					Type: schema.Integer,
					Desc: "Maximum number of results to return, default 5",
				},
			}),
		},
	}
}

// CallTool 调用 web_search 工具
func (p *WebSearchToolProvider) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	if name != "web_search" {
		return "", fmt.Errorf("unknown tool: %s", name)
	}

	query, ok := args["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("query is required")
	}

	maxResults := 5
	if mr, ok := args["max_results"].(float64); ok {
		maxResults = int(mr)
	}

	resp, err := p.tool.Run(ctx, &SearchRequest{
		Query:      query,
		MaxResults: maxResults,
	})
	if err != nil {
		return "", err
	}

	data, _ := json.Marshal(resp)
	return string(data), nil
}

// ============================================================================
// 工具适配器 - 将 ToolProvider 适配为 eino tool.BaseTool
// ============================================================================

// ToolProviderAdapter 将 ToolProvider 适配为 eino tool
type ToolProviderAdapter struct {
	name     string
	provider ToolProvider
}

// Info 返回工具信息
func (t *ToolProviderAdapter) Info(ctx context.Context) (*schema.ToolInfo, error) {
	tools := t.provider.GetTools()
	for _, ti := range tools {
		if ti.Name == t.name {
			return ti, nil
		}
	}
	return nil, fmt.Errorf("tool %s not found", t.name)
}

// InvokableRun 执行工具
func (t *ToolProviderAdapter) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args map[string]any
	if argumentsInJSON != "" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
			return "", fmt.Errorf("failed to unmarshal arguments: %w", err)
		}
	}
	return t.provider.CallTool(ctx, t.name, args)
}
