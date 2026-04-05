/*
 * Video Script Generation - MCP Client
 * MCP 客户端实现，用于调用外部 MCP 服务器的工具
 */

package tools

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
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// SearchResult 在 web_search.go 中定义
// 这里复用其定义，通过 MCP 返回的搜索结果

// MCPClient MCP 客户端
type MCPClient struct {
	name  string
	cmd   string
	env   []string
	tools []mcp.Tool // 已连接的工具列表
}

// NewMCPClient 创建 MCP 客户端
func NewMCPClient(name, cmd string, env []string) *MCPClient {
	return &MCPClient{
		name:  name,
		cmd:   cmd,
		env:   env,
		tools: make([]mcp.Tool, 0),
	}
}

// ListTools 返回已连接的工具列表
func (c *MCPClient) ListTools() []mcp.Tool {
	return c.tools
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
			ProtocolVersion string                 `json:"protocolVersion"`
			Capabilities    mcp.ClientCapabilities `json:"capabilities"`
			ClientInfo      mcp.Implementation     `json:"clientInfo"`
		}{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "clawgo-video",
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

	// 读取响应
	var toolsResult []mcp.Tool
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

		//log.Printf("MCP raw response: %s", string(line))

		var resp mcp.JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			log.Printf("Failed to unmarshal response: %v", err)
			continue
		}

		if resp.Result != nil {
			resultBytes, _ := json.Marshal(resp.Result)
			id, _ := resp.ID.(float64)
			if id == 2 {
				var result mcp.ListToolsResult
				if err := json.Unmarshal(resultBytes, &result); err == nil {
					toolsResult = result.Tools
				}
			}
		}
		if toolsResult != nil {
			break
		}
	}

	cmd.Wait()

	c.tools = toolsResult
	// log.Printf("Connected to MCP server: %s, found %d tools", c.name, len(c.tools))
	// for _, t := range c.tools {
	// 	log.Printf("  - %s: %s", t.Name, t.Description)
	// }

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

// ListToolsFromServer 列出可用工具（连接服务器获取）
func (c *MCPClient) ListToolsFromServer(ctx context.Context) ([]mcp.Tool, error) {
	// 构建命令
	parts, err := parseCommand(c.cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to parse command: %w", err)
	}

	// 构建环境变量
	mergedEnv := os.Environ()
	if c.env != nil {
		mergedEnv = append(mergedEnv, c.env...)
	}

	// 启动进程
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Env = mergedEnv

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start MCP server: %w", err)
	}

	// 发送初始化请求
	initializeReq := mcp.JSONRPCRequest{
		JSONRPC: mcp.JSONRPC_VERSION,
		ID:      1,
		Request: mcp.Request{
			Method: "initialize",
		},
		Params: struct {
			ProtocolVersion string                 `json:"protocolVersion"`
			Capabilities    mcp.ClientCapabilities `json:"capabilities"`
			ClientInfo      mcp.Implementation     `json:"clientInfo"`
		}{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "clawgo-video",
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

	// 读取响应
	var toolsResult []mcp.Tool
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

		//log.Printf("MCP raw response: %s", string(line))

		var resp mcp.JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			log.Printf("Failed to unmarshal response: %v", err)
			continue
		}

		if resp.Result != nil {
			resultBytes, _ := json.Marshal(resp.Result)
			id, _ := resp.ID.(float64)
			if id == 2 {
				var result mcp.ListToolsResult
				if err := json.Unmarshal(resultBytes, &result); err == nil {
					toolsResult = result.Tools
				}
			}
		}
		if toolsResult != nil {
			break
		}
	}

	cmd.Wait()
	//log.Printf("Connected to MCP server: %s, found %d tools", c.name, len(toolsResult))
	return toolsResult, nil
}

// CallTool 调用工具
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

	// 发送初始化请求
	initializeReq := mcp.JSONRPCRequest{
		JSONRPC: mcp.JSONRPC_VERSION,
		ID:      1,
		Request: mcp.Request{
			Method: "initialize",
		},
		Params: struct {
			ProtocolVersion string                 `json:"protocolVersion"`
			Capabilities    mcp.ClientCapabilities `json:"capabilities"`
			ClientInfo      mcp.Implementation     `json:"clientInfo"`
		}{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "clawgo-video",
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
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments,omitempty"`
		}{
			Name:      toolName,
			Arguments: arguments,
		},
	}
	callBytes, _ := json.Marshal(callToolReq)
	stdin.Write(append(callBytes, '\n'))
	stdin.Close()

	// 设置超时（图片理解等任务可能需要更长时间）
	timeout := time.AfterFunc(120*time.Second, func() {
		log.Printf("MCP call timeout (120s), killing process")
		cmd.Process.Kill()
	})
	defer timeout.Stop()

	// 读取所有输出
	output, err := io.ReadAll(stdout)
	if err != nil {
		return "", fmt.Errorf("failed to read stdout: %w", err)
	}

	// 按行分割处理
	var callResult map[string]interface{}
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

		id, _ := resp.ID.(float64)
		if id != 2 {
			continue
		}

		if resp.Result != nil {
			resultBytes, _ := json.Marshal(resp.Result)
			if err := json.Unmarshal(resultBytes, &callResult); err == nil {
				break
			}
		}
	}

	cmd.Wait()

	if callResult == nil {
		return "", fmt.Errorf("failed to get tool call result")
	}

	// 提取 content
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

// Search 调用 web_search 工具（通过 MCP）
func (c *MCPClient) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	result, err := c.CallTool(ctx, "web_search", map[string]any{
		"query":       query,
		"max_results": maxResults,
	})
	if err != nil {
		return nil, err
	}

	// 解析结果
	var searchResults []SearchResult

	// 尝试 JSON 解析
	if err := json.Unmarshal([]byte(result), &searchResults); err != nil {
		// 如果不是 JSON，尝试解析为纯文本搜索结果
		lines := strings.Split(result, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			searchResults = append(searchResults, SearchResult{
				Title:   line,
				Snippet: "",
			})
		}
	}

	return searchResults, nil
}
