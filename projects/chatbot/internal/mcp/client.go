/*
 * Chat Bot Management System - MCP Client
 * MCP 客户端实现，支持单次请求模式的 MCP 服务器
 */

package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// MCPClient MCP 客户端（支持单次请求模式）
type MCPClient struct {
	name  string
	cmd   string
	args  []string
	env   []string
	tools []mcp.Tool
}

// NewMCPClient 创建 MCP 客户端
func NewMCPClient(name, cmd string, args []string, env []string) *MCPClient {
	return &MCPClient{
		name: name,
		cmd:  cmd,
		args: args,
		env:  env,
	}
}

// Connect 连接 MCP 服务器并获取工具列表
func (c *MCPClient) Connect(ctx context.Context) error {
	log.Printf("Connecting to MCP server: %s (cmd: %s)", c.name, c.cmd)

	// 构建命令
	parts, err := parseCommand(c.cmd)
	if err != nil {
		return fmt.Errorf("failed to parse command: %w", err)
	}

	// 构建环境变量
	mergedEnv := os.Environ()
	if c.env != nil {
		mergedEnv = append(mergedEnv, c.env...)
	}

	// 启动进程进行初始化
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Env = mergedEnv

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start MCP server: %w", err)
	}

	// 发送初始化请求
	initializeReq := mcp.JSONRPCRequest{
		JSONRPC: mcp.JSONRPC_VERSION,
		ID:      1,
		Request: mcp.Request{
			Method: "initialize",
		},
		Params: struct {
			ProtocolVersion string                   `json:"protocolVersion"`
			Capabilities    mcp.ClientCapabilities `json:"capabilities"`
			ClientInfo      mcp.Implementation     `json:"clientInfo"`
		}{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "clawgo-chatbot",
				Version: "1.0.0",
			},
		},
	}

	reqBytes, _ := json.Marshal(initializeReq)
	stdin.Write(append(reqBytes, '\n'))

	// 发送 initialized 通知
	initializedNotif := mcp.JSONRPCNotification{
		JSONRPC: mcp.JSONRPC_VERSION,
		Notification: mcp.Notification{
			Method: "notifications/initialized",
		},
	}
	notifBytes, _ := json.Marshal(initializedNotif)
	stdin.Write(append(notifBytes, '\n'))

	// 发送工具列表请求
	listToolsReq := mcp.JSONRPCRequest{
		JSONRPC: mcp.JSONRPC_VERSION,
		ID:      2,
		Request: mcp.Request{
			Method: "tools/list",
		},
	}
	listBytes, _ := json.Marshal(listToolsReq)
	stdin.Write(append(listBytes, '\n'))
	stdin.Close()

	// 读取响应（给服务器时间处理）
	var toolsResult *mcp.ListToolsResult
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			break
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		log.Printf("MCP raw response: %s", string(line))

		var resp mcp.JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			log.Printf("Failed to unmarshal response: %v", err)
			continue
		}

		if resp.Result != nil {
			resultBytes, _ := json.Marshal(resp.Result)

			// 根据 ID 判断是哪个响应（JSON numbers unmarshal as float64）
			id, _ := resp.ID.(float64)
			if id == 2 {
				var result mcp.ListToolsResult
				if err := json.Unmarshal(resultBytes, &result); err == nil {
					toolsResult = &result
				}
			}
		}
		if toolsResult != nil {
			break
		}
	}

	cmd.Wait()

	if toolsResult != nil {
		c.tools = toolsResult.Tools
	}

	log.Printf("Connected to MCP server: %s, found %d tools", c.name, len(c.tools))
	for _, t := range c.tools {
		log.Printf("  - %s: %s", t.Name, t.Description)
	}

	return nil
}

// parseCommand 解析命令字符串
func parseCommand(cmd string) ([]string, error) {
	var parts []string
	var current string
	inQuotes := false
	quoteChar := byte(0)

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if c == '"' || c == '\'' {
			if !inQuotes {
				inQuotes = true
				quoteChar = c
			} else if c == quoteChar {
				inQuotes = false
			} else {
				current += string(c)
			}
		} else if c == ' ' && !inQuotes {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}

	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return parts, nil
}

// Close 关闭连接（单次模式无需关闭）
func (c *MCPClient) Close() error {
	return nil
}

// ListTools 列出可用工具
func (c *MCPClient) ListTools() []mcp.Tool {
	return c.tools
}

// CallTool 调用工具（每次调用启动新进程）
func (c *MCPClient) CallTool(ctx context.Context, toolName string, arguments map[string]any) (string, error) {
	// 构建命令
	parts, err := parseCommand(c.cmd)
	if err != nil {
		return "", fmt.Errorf("failed to parse command: %w", err)
	}

	// 构建环境变量
	mergedEnv := os.Environ()
	if c.env != nil {
		mergedEnv = append(mergedEnv, c.env...)
	}

	// 启动新进程
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Env = mergedEnv

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start MCP server: %w", err)
	}

	// 调试：打印环境变量
	log.Printf("MCP env: %v", c.env)

	// 发送初始化请求
	initializeReq := mcp.JSONRPCRequest{
		JSONRPC: mcp.JSONRPC_VERSION,
		ID:      1,
		Request: mcp.Request{
			Method: "initialize",
		},
		Params: struct {
			ProtocolVersion string                   `json:"protocolVersion"`
			Capabilities    mcp.ClientCapabilities `json:"capabilities"`
			ClientInfo      mcp.Implementation     `json:"clientInfo"`
		}{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "clawgo-chatbot",
				Version: "1.0.0",
			},
		},
	}
	reqBytes, _ := json.Marshal(initializeReq)
	stdin.Write(append(reqBytes, '\n'))

	// 发送 initialized 通知
	initializedNotif := mcp.JSONRPCNotification{
		JSONRPC: mcp.JSONRPC_VERSION,
		Notification: mcp.Notification{
			Method: "notifications/initialized",
		},
	}
	notifBytes, _ := json.Marshal(initializedNotif)
	stdin.Write(append(notifBytes, '\n'))

	// 发送工具调用请求
	callToolReq := mcp.JSONRPCRequest{
		JSONRPC: mcp.JSONRPC_VERSION,
		ID:      2,
		Request: mcp.Request{
			Method: "tools/call",
		},
		Params: struct {
			Name      string                  `json:"name"`
			Arguments map[string]interface{}  `json:"arguments,omitempty"`
		}{
			Name:      toolName,
			Arguments: arguments,
		},
	}
	callBytes, _ := json.Marshal(callToolReq)
	stdin.Write(append(callBytes, '\n'))
	stdin.Close()

	// 读取响应
	var callResult map[string]interface{}
	var respID float64

	// 设置超时
	timeout := time.AfterFunc(30*time.Second, func() {
		log.Printf("MCP call timeout, killing process")
		cmd.Process.Kill()
	})
	defer timeout.Stop()

	// 读取所有输出
	output, err := io.ReadAll(stdout)
	if err != nil {
		return "", fmt.Errorf("failed to read stdout: %w", err)
	}

	// 按行分割处理
	lines := bytes.Split(output, []byte{'\n'})
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		log.Printf("MCP call raw response: %s", string(line))

		var resp mcp.JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			log.Printf("Failed to unmarshal response: %v", err)
			continue
		}

		// 提取 ID
		switch v := resp.ID.(type) {
		case float64:
			respID = v
		case int:
			respID = float64(v)
		case json.Number:
			respID, _ = v.Float64()
		}

		// 检查 ID 是否为 2（tools/call 的响应）
		if respID != 2 {
			continue
		}

		if resp.Result != nil {
			resultBytes, _ := json.Marshal(resp.Result)
			log.Printf("MCP call result: %s", string(resultBytes))
			if err := json.Unmarshal(resultBytes, &callResult); err == nil {
				break
			} else {
				log.Printf("Failed to unmarshal call result: %v", err)
			}
		}
	}

	cmd.Wait()

	if callResult == nil {
		return "", fmt.Errorf("failed to get tool call result")
	}

	// 从 callResult 直接提取 content（避免 mcp.CallToolResult Content 接口类型 unmarshal 问题）
	contentSlice, ok := callResult["content"].([]interface{})
	if !ok || len(contentSlice) == 0 {
		return "", fmt.Errorf("no content in result")
	}

	contentMap, ok := contentSlice[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid content format")
	}

	if text, ok := contentMap["text"].(string); ok && text != "" {
		return text, nil
	}

	return "", fmt.Errorf("no text in content")
}

// GetName 获取客户端名称
func (c *MCPClient) GetName() string {
	return c.name
}
