/*
 * Chat Bot Management System - MCP Server
 * MCP 服务器实现，通过 stdio 传输暴露工具
 */

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ToolHandler MCP 工具处理函数类型
type ToolHandler func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)

// MCPServer MCP 服务器
type MCPServer struct {
	name   string
	srv    *server.MCPServer
	tools  map[string]ToolHandler
}

// NewMCPServer 创建 MCP 服务器
func NewMCPServer(name, version string) *MCPServer {
	srv := server.NewMCPServer(name, version)
	return &MCPServer{
		name:  name,
		srv:   srv,
		tools: make(map[string]ToolHandler),
	}
}

// Tool 定义 MCP 工具
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// RegisterTool 注册工具
func (s *MCPServer) RegisterTool(tool Tool, handler ToolHandler) error {
	s.tools[tool.Name] = handler

	schema := mcp.ToolInputSchema{
		Type:       "object",
		Properties: make(map[string]any),
	}
	if tool.InputSchema != nil {
		for k, v := range tool.InputSchema {
			schema.Properties[k] = v
		}
	}

	mcpTool := mcp.NewTool(tool.Name,
		mcp.WithDescription(tool.Description),
	)
	mcpTool.InputSchema = schema

	s.srv.AddTool(mcpTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		h, ok := s.tools[req.Params.Name]
		if !ok {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{Type: "text", Text: fmt.Sprintf("tool %s not found", req.Params.Name)},
				},
			}, nil
		}

		args := req.Params.Arguments
		var argsJSON json.RawMessage
		if args != nil {
			argsJSON, _ = json.Marshal(args)
		} else {
			argsJSON = json.RawMessage("{}")
		}

		result, err := h(ctx, argsJSON)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{Type: "text", Text: fmt.Sprintf("error: %v", err)},
				},
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: string(result)},
			},
		}, nil
	})

	return nil
}

// Start 启动 MCP 服务器
func (s *MCPServer) Start(ctx context.Context) error {
	log.Printf("Starting MCP server: %s", s.name)

	if err := server.ServeStdio(s.srv); err != nil {
		return fmt.Errorf("MCP server error: %w", err)
	}

	return nil
}
