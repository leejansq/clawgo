/*
 * Copyright 2026 CloudWeGo Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/leejansq/clawgo/pkg/types"
)

// executeSubAgentAsync 后台异步执行子 Agent
func (m *SubAgentManager) executeSubAgentAsync(ctx context.Context, sessionKey string, config *types.SubAgentConfig, workspacePath string) {
	startTime := time.Now()
	fmt.Printf("│ ⚡ [SUBAGENT START] session=%s, label=%s, task=%s\n", sessionKey, config.Label, config.Task)

	// 更新状态为 running
	m.mu.Lock()
	if info, ok := m.sessions[sessionKey]; ok {
		info.Status = "running"
		fmt.Printf("│ ⚡ [STATUS] Updated to 'running'\n")
	}
	m.depth++
	m.mu.Unlock()

	var result string

	// 根据配置选择执行方式
	if m.useClaudeCLI {
		// 使用 Claude Code CLI 执行
		fmt.Printf("│ ⚡ [EXEC MODE] Using Claude CLI\n")
		result = m.executeWithClaudeCLI(ctx, sessionKey, config, workspacePath, startTime)
	} else {
		// 使用 Eino ChatModel 执行
		fmt.Printf("│ ⚡ [EXEC MODE] Using ChatModel\n")
		result = m.executeWithChatModel(ctx, sessionKey, config, workspacePath, startTime)
	}

	// 保存到 ResultStore (供 get_subagent_result 查询)
	info := &types.SubAgentInfo{
		SessionKey:    sessionKey,
		Label:         config.Label,
		Task:          config.Task,
		Status:        "completed",
		Result:        result,
		DurationMs:    time.Since(startTime).Milliseconds(),
		WorkspacePath: workspacePath,
		Model:         config.Model,
	}
	m.resultStore.Save(info)

	// 发送 Announce 通知 (参考 OpenClaw push 模式)
	m.EmitAnnounce(&types.Announce{
		SessionKey: sessionKey,
		Status:     "completed",
		Result:     result,
		Label:      config.Label,
		DurationMs: time.Since(startTime).Milliseconds(),
	})

	// 清理
	m.saveAndDecrementDepth(sessionKey, config.Cleanup, workspacePath)

	fmt.Printf("\n✅ [ASYNC COMPLETED] Sub-agent %s finished in %dms\n", config.Label, time.Since(startTime).Milliseconds())
}

// executeWithChatModel 使用 Eino ChatModel 执行子任务
func (m *SubAgentManager) executeWithChatModel(ctx context.Context, sessionKey string, config *types.SubAgentConfig, workspacePath string, startTime time.Time) string {
	// 构建子 Agent 的 system prompt
	systemPrompt := buildSubAgentSystemPrompt(config.Instruction, workspacePath)

	// 创建带超时的上下文
	subCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	// 调用模型执行子任务
	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(config.Task),
	}

	stream, err := m.cm.Stream(subCtx, messages)
	if err != nil {
		m.updateResult(sessionKey, "error", "", err.Error(), startTime)
		// 保存到 ResultStore
		m.resultStore.Save(&types.SubAgentInfo{
			SessionKey:    sessionKey,
			Label:         config.Label,
			Task:          config.Task,
			Status:        "error",
			Error:         err.Error(),
			DurationMs:    time.Since(startTime).Milliseconds(),
			WorkspacePath: workspacePath,
		})
		// 发送 Announce 通知错误
		m.EmitAnnounce(&types.Announce{
			SessionKey: sessionKey,
			Status:     "error",
			Error:      err.Error(),
			Label:      config.Label,
		})
		m.saveAndDecrementDepth(sessionKey, config.Cleanup, workspacePath)
		return ""
	}
	defer stream.Close()

	// 收集输出
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
	if result == "" {
		result = "No output from sub-agent"
	}

	// 更新结果
	m.updateResult(sessionKey, "completed", result, "", startTime)

	return result
}

// executeWithClaudeCLI 使用 Claude Code CLI 执行子任务
func (m *SubAgentManager) executeWithClaudeCLI(ctx context.Context, sessionKey string, config *types.SubAgentConfig, workspacePath string, startTime time.Time) string {
	fmt.Printf("│ 🔥 [CLAUDE CLI] Starting...\n")

	// 确定模式: run (one-shot) 或 session (persistent)
	mode := os.Getenv("CLAUDE_SUBAGENT_MODE")
	if mode == "" {
		mode = "run" // 默认 one-shot 模式
	}
	fmt.Printf("│ 🔥 [CLAUDE CLI] Mode: %s\n", mode)

	// 构建子 Agent 的 system prompt
	systemPrompt := buildSubAgentSystemPrompt(config.Instruction, workspacePath)
	fmt.Printf("│ 🔥 [CLAUDE CLI] System prompt: %s\n", systemPrompt[:min(100, len(systemPrompt))])

	// 创建带超时的上下文
	subCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	// 获取或创建 Claude Runner (session 模式会复用已有进程)
	runner, _, err := m.GetOrCreateClaudeRunner(workspacePath, systemPrompt, config.Task)
	if err != nil {
		errMsg := fmt.Sprintf("failed to create Claude runner: %v", err)
		fmt.Printf("│ ❌ [CLAUDE CLI] Failed to create runner: %v\n", err)
		m.updateResult(sessionKey, "error", "", errMsg, startTime)
		// 保存到 ResultStore
		m.resultStore.Save(&types.SubAgentInfo{
			SessionKey:    sessionKey,
			Label:         config.Label,
			Task:          config.Task,
			Status:        "error",
			Error:         errMsg,
			DurationMs:    time.Since(startTime).Milliseconds(),
			WorkspacePath: workspacePath,
		})
		// 发送 Announce 通知错误
		m.EmitAnnounce(&types.Announce{
			SessionKey: sessionKey,
			Status:     "error",
			Error:      errMsg,
			Label:      config.Label,
		})
		m.saveAndDecrementDepth(sessionKey, config.Cleanup, workspacePath)
		return ""
	}
	defer runner.Stop()

	// 执行 Claude Code
	fmt.Printf("│ 🔥 [CLAUDE CLI] Running in workspace: %s\n", workspacePath)

	// 根据模式选择执行方式
	var result string
	if mode == "session" {
		result, err = runner.RunSession(subCtx, config.Task)
	} else {
		result, err = runner.Run(subCtx)
	}

	if err != nil {
		errMsg := fmt.Sprintf("failed to run Claude: %v", err)
		fmt.Printf("│ ❌ [CLAUDE CLI] Failed to run: %v\n", err)
		m.updateResult(sessionKey, "error", "", errMsg, startTime)
		m.resultStore.Save(&types.SubAgentInfo{
			SessionKey:    sessionKey,
			Label:         config.Label,
			Task:          config.Task,
			Status:        "error",
			Error:         errMsg,
			DurationMs:    time.Since(startTime).Milliseconds(),
			WorkspacePath: workspacePath,
		})
		m.EmitAnnounce(&types.Announce{
			SessionKey: sessionKey,
			Status:     "error",
			Error:      errMsg,
			Label:      config.Label,
		})
		m.saveAndDecrementDepth(sessionKey, config.Cleanup, workspacePath)
		return ""
	}

	// 更新结果
	m.updateResult(sessionKey, "completed", result, "", startTime)

	return result
}

func (m *SubAgentManager) updateResult(sessionKey, status, result, errMsg string, startTime time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if info, ok := m.sessions[sessionKey]; ok {
		info.Status = status
		info.Result = result
		if errMsg != "" {
			info.Error = errMsg
		}
		now := time.Now()
		info.CompletedAt = &now
		info.DurationMs = time.Since(startTime).Milliseconds()
	}
}

func (m *SubAgentManager) saveAndDecrementDepth(sessionKey, cleanup, workspacePath string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.depth--

	// 根据 cleanup 选项决定是否清理工作区
	if cleanup == "delete" {
		fmt.Printf("│ 🗑️ [CLEANUP] Deleting workspace: %s\n", workspacePath)
		os.RemoveAll(workspacePath)
	} else {
		fmt.Printf("│ 💾 [CLEANUP] Keeping workspace: %s\n", workspacePath)
	}
}

// buildSubAgentSystemPrompt 构建子 Agent 的 system prompt
func buildSubAgentSystemPrompt(instruction, workspacePath string) string {
	prompt := `You are a sub-agent responsible for completing a specific task.

Your capabilities:
- Read, write, and edit files
- Execute shell commands
- Search for files and content
- Use tools to accomplish your task

Workspace: ` + workspacePath + `

Instructions:
1. First understand the task
2. Plan your approach
3. Execute the plan step by step
4. Report the result

When you finish, provide a summary of what you did.`

	if instruction != "" {
		prompt += "\n\nAdditional instructions:\n" + instruction
	}

	return prompt
}
