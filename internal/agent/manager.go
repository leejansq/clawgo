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
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/google/uuid"
	"github.com/leejansq/clawgo/pkg/types"
)

// SubAgentManager 子 Agent 管理器 (参考 OpenClaw subagent-registry)
type SubAgentManager struct {
	mu                sync.RWMutex
	sessions          map[string]*types.SubAgentInfo
	cm                model.ChatModel
	workspaceRoot     string
	resultStore       *ResultStore
	depth             int
	maxDepth          int
	useClaudeCLI      bool   // 是否使用 Claude Code CLI 作为子 Agent
	existingWorkspace string // 指定的工作区（用于现有项目开发）

	// Claude CLI 进程池 (用于 session 模式复用)
	claudeRunners map[string]*ClaudeRunner // key: workspace path

	// Announce 机制 (参考 OpenClaw subagent-announce)
	announceChan chan *types.Announce // 子 Agent 完成通知通道
}

// NewSubAgentManager 创建子 Agent 管理器
func NewSubAgentManager(cm model.ChatModel, workspaceRoot string, maxDepth int) *SubAgentManager {
	if maxDepth == 0 {
		maxDepth = 5
	}
	return &SubAgentManager{
		sessions:       make(map[string]*types.SubAgentInfo),
		cm:             cm,
		workspaceRoot:  workspaceRoot,
		resultStore:    NewResultStore(workspaceRoot),
		depth:          0,
		maxDepth:       maxDepth,
		useClaudeCLI:    os.Getenv("USE_CLAUDE_CLI") == "true",
		claudeRunners:  make(map[string]*ClaudeRunner), // Claude CLI 进程池
		announceChan:   make(chan *types.Announce, 10), // Announce 通道
	}
}

// GetOrCreateClaudeRunner 获取或创建 Claude Runner (用于 session 模式复用)
func (m *SubAgentManager) GetOrCreateClaudeRunner(workspace, systemPrompt, task string) (*ClaudeRunner, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	mode := os.Getenv("CLAUDE_SUBAGENT_MODE")
	if mode == "" {
		mode = "run"
	}

	// session 模式：检查是否有可复用的 Runner
	if mode == "session" {
		if runner, exists := m.claudeRunners[workspace]; exists {
			if runner.IsRunning() {
				fmt.Printf("│ 🔄 [REUSE] Reusing Claude process for workspace: %s\n", workspace)
				return runner, true, nil
			}
			// 进程已停止，删除旧的
			delete(m.claudeRunners, workspace)
		}
	}

	// 创建新的 Runner
	cliSessionID := generateUUID()
	runner, err := NewClaudeRunner(workspace, cliSessionID, systemPrompt, task, mode)
	if err != nil {
		return nil, false, err
	}

	// session 模式：跟踪 Runner 用于后续复用
	if mode == "session" {
		m.claudeRunners[workspace] = runner
	}
	fmt.Printf("│ 🆕 [NEW] Created new Claude process for workspace: %s (mode=%s)\n", workspace, mode)

	return runner, false, nil
}

// StopAllClaudeRunners 停止所有 Claude Runner (整体任务结束时调用)
func (m *SubAgentManager) StopAllClaudeRunners() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for workspace, runner := range m.claudeRunners {
		fmt.Printf("│ 🛑 [STOP] Stopping Claude process for workspace: %s\n", workspace)
		runner.Stop()
	}
	m.claudeRunners = make(map[string]*ClaudeRunner)
}

// SetExistingWorkspace 设置现有工作区（用于在现有项目上进行开发）
func (m *SubAgentManager) SetExistingWorkspace(workspace string) {
	m.existingWorkspace = workspace
}

// GetExistingWorkspace 获取现有工作区
func (m *SubAgentManager) GetExistingWorkspace() string {
	return m.existingWorkspace
}

// GetAnnounceChan 获取 Announce 通知通道 (用于 Gateway SSE)
func (m *SubAgentManager) GetAnnounceChan() chan *types.Announce {
	return m.announceChan
}

// EmitAnnounce 发送 Announce 通知 (参考 OpenClaw runSubagentAnnounceFlow)
func (m *SubAgentManager) EmitAnnounce(announce *types.Announce) {
	select {
	case m.announceChan <- announce:
		fmt.Printf("│ 📬 [ANNOUNCE] Sent: session=%s, status=%s\n", announce.SessionKey, announce.Status)
	default:
		fmt.Printf("│ ⚠️  [ANNOUNCE] Channel full, dropping: session=%s\n", announce.SessionKey)
	}
}

// WaitForAnnounce 等待 Announce 通知 (阻塞等待)
func (m *SubAgentManager) WaitForAnnounce(timeout time.Duration) *types.Announce {
	select {
	case announce := <-m.announceChan:
		return announce
	case <-time.After(timeout):
		return nil
	}
}

// PollAnnounce 非阻塞获取 Announce
func (m *SubAgentManager) PollAnnounce() *types.Announce {
	select {
	case announce := <-m.announceChan:
		return announce
	default:
		return nil
	}
}

// GetResult 获取结果
func (m *SubAgentManager) GetResult(sessionKey string) *types.SubAgentInfo {
	return m.resultStore.Get(sessionKey)
}

// ListActiveSessions 列出活跃会话
func (m *SubAgentManager) ListActiveSessions() []*types.SubAgentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*types.SubAgentInfo, 0, len(m.sessions))
	for _, info := range m.sessions {
		result = append(result, info)
	}
	return result
}

// createWorkspace 创建工作区
func (m *SubAgentManager) createWorkspace(sessionKey, label string) (string, error) {
	workspacePath := filepath.Join(m.workspaceRoot, sessionKey)
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return "", fmt.Errorf("failed to create workspace: %w", err)
	}

	// 记录初始文件列表（用于后续检测改动）
	initialFiles, err := recordInitialFiles(workspacePath)
	if err != nil {
		fmt.Printf("│ ⚠️ [WORKSPACE] Failed to record initial files: %v\n", err)
	}

	// 记录到 SubAgentInfo 中
	info := &types.SubAgentInfo{
		SessionKey:       sessionKey,
		WorkspacePath:    workspacePath,
		InitialFiles:     initialFiles,
		InitialFilesHash: hashFiles(initialFiles),
		CreatedAt:        time.Now(),
	}
	m.sessions[sessionKey] = info

	fmt.Printf("│ 📁 [WORKSPACE] Created workspace: %s (initial files: %d)\n", workspacePath, len(initialFiles))

	return workspacePath, nil
}

// SpawnAsyncInWorkspace 在指定工作区中创建子 Agent (共享 workspace)
func (m *SubAgentManager) SpawnAsyncInWorkspace(ctx context.Context, config *types.SubAgentConfig, existingWorkspacePath string) *types.SubAgentInfo {
	// 检查深度限制
	if m.depth >= m.maxDepth {
		errInfo := &types.SubAgentInfo{
			SessionKey: generateSessionKey(config.Label),
			Label:      config.Label,
			Status:     "error",
			Error:      fmt.Sprintf("max sub-agent depth (%d) exceeded", m.maxDepth),
		}
		m.EmitAnnounce(&types.Announce{
			SessionKey: errInfo.SessionKey,
			Status:     "error",
			Error:      errInfo.Error,
			Label:      config.Label,
		})
		return errInfo
	}

	// 生成唯一会话 key
	sessionKey := generateSessionKey(config.Label)

	// 使用已存在的工作区
	workspacePath := existingWorkspacePath

	// 打印 Spawn 信息 (立即返回)
	fmt.Printf("\n┌─────────────────────────────────────────────────────────────────────┐\n")
	fmt.Printf("│ 🤖 [SPAWN SHARED] Sub-Agent: %s (shared workspace)\n", config.Label)
	fmt.Printf("│ 🔑 [SESSION] %s\n", sessionKey)
	fmt.Printf("│ 📁 [WORKSPACE] %s (shared)\n", workspacePath)
	fmt.Printf("│ ⏱️  [TIMEOUT] %v\n", config.Timeout)
	fmt.Printf("│ 📬 [MODE] Async - results will be available via get_subagent_result\n")
	fmt.Printf("└─────────────────────────────────────────────────────────────────────┘\n\n")

	// 创建子 Agent info (状态: pending)
	info := &types.SubAgentInfo{
		SessionKey:    sessionKey,
		Label:         config.Label,
		Task:          config.Task,
		Status:        "pending",
		CreatedAt:     time.Now(),
		WorkspacePath: workspacePath,
		Model:         config.Model,
	}

	// 注册会话
	m.mu.Lock()
	m.sessions[sessionKey] = info
	m.mu.Unlock()

	// 在后台异步执行 (不阻塞工具调用)
	go m.executeSubAgentAsync(ctx, sessionKey, config, workspacePath)

	return info
}

// SpawnAsync 异步 Spawn (立即返回，不等待完成) - 核心功能
func (m *SubAgentManager) SpawnAsync(ctx context.Context, config *types.SubAgentConfig) *types.SubAgentInfo {
	// 检查深度限制
	if m.depth >= m.maxDepth {
		errInfo := &types.SubAgentInfo{
			SessionKey: generateSessionKey(config.Label),
			Label:      config.Label,
			Status:     "error",
			Error:      fmt.Sprintf("max sub-agent depth (%d) exceeded", m.maxDepth),
		}
		// 发送 Announce 通知错误
		m.EmitAnnounce(&types.Announce{
			SessionKey: errInfo.SessionKey,
			Status:     "error",
			Error:      errInfo.Error,
			Label:      config.Label,
		})
		return errInfo
	}

	// 生成唯一会话 key
	sessionKey := generateSessionKey(config.Label)

	// 确定工作区路径
	var workspacePath string
	var err error

	if m.existingWorkspace != "" {
		// 使用现有的工作区
		workspacePath = m.existingWorkspace
		fmt.Printf("│ 📁 [WORKSPACE] Using existing workspace: %s\n", workspacePath)
	} else {
		// 创建新工作区
		workspacePath, err = m.createWorkspace(sessionKey, config.Label)
		if err != nil {
			errInfo := &types.SubAgentInfo{
				SessionKey: sessionKey,
				Label:      config.Label,
				Status:     "error",
				Error:      err.Error(),
			}
			// 发送 Announce 通知错误
			m.EmitAnnounce(&types.Announce{
				SessionKey: sessionKey,
				Status:     "error",
				Error:      err.Error(),
				Label:      config.Label,
			})
			return errInfo
		}
	}

	// 打印 Spawn 信息 (立即返回)
	fmt.Printf("\n┌─────────────────────────────────────────────────────────────────────┐\n")
	fmt.Printf("│ 🤖 [SPAWN ASYNC] Sub-Agent: %s\n", config.Label)
	fmt.Printf("│ 🔑 [SESSION] %s\n", sessionKey)
	fmt.Printf("│ 📁 [WORKSPACE] %s\n", workspacePath)
	fmt.Printf("│ ⏱️  [TIMEOUT] %v\n", config.Timeout)
	fmt.Printf("│ 📬 [MODE] Async - results will be available via get_subagent_result\n")
	fmt.Printf("└─────────────────────────────────────────────────────────────────────┘\n\n")

	// 创建子 Agent info (状态: pending)
	info := &types.SubAgentInfo{
		SessionKey:    sessionKey,
		Label:         config.Label,
		Task:          config.Task,
		Status:        "pending",
		CreatedAt:     time.Now(),
		WorkspacePath: workspacePath,
		Model:         config.Model,
	}

	// 注册会话
	m.mu.Lock()
	m.sessions[sessionKey] = info
	m.mu.Unlock()

	// 在后台异步执行 (不阻塞工具调用)
	go m.executeSubAgentAsync(ctx, sessionKey, config, workspacePath)

	return info
}

// Helper functions
func generateSessionKey(label string) string {
	return fmt.Sprintf("subagent:%s:%s", label, uuid.New().String())
}

func generateUUID() string {
	return uuid.New().String()
}

func getFileExtension(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return "Go"
	case ".js", ".jsx", ".ts", ".tsx":
		return "JavaScript/TypeScript"
	case ".py":
		return "Python"
	case ".java":
		return "Java"
	case ".cpp", ".cc", ".c":
		return "C/C++"
	case ".rs":
		return "Rust"
	case ".rb":
		return "Ruby"
	case ".php":
		return "PHP"
	case ".swift":
		return "Swift"
	case ".kt", ".kts":
		return "Kotlin"
	default:
		return strings.TrimPrefix(ext, ".")
	}
}

func recordInitialFiles(workspacePath string) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(workspacePath)
	if err != nil {
		return files, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

func hashFiles(files []string) string {
	h := sha256.New()
	for _, f := range files {
		h.Write([]byte(f))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
