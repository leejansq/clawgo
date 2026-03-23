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

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

// ============================================================================
// 核心类型定义 (参考 OpenClaw)
// ============================================================================

// SubAgentInfo 子 Agent 信息 (参考 OpenClaw SubAgentRunRecord)
type SubAgentInfo struct {
	SessionKey       string     `json:"sessionKey"`
	Label            string     `json:"label"`
	Task             string     `json:"task"`
	Status           string     `json:"status"` // pending, running, completed, error
	Result           string     `json:"result,omitempty"`
	Error            string     `json:"error,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	CompletedAt      *time.Time `json:"completedAt,omitempty"`
	DurationMs       int64      `json:"durationMs"`
	WorkspacePath    string     `json:"workspacePath,omitempty"`
	Model            string     `json:"model,omitempty"`
	InitialFiles     []string   `json:"initialFiles,omitempty"`     // 初始文件列表（用于检测改动）
	InitialFilesHash string     `json:"initialFilesHash,omitempty"` // 初始文件 hash
}

// SubAgentConfig 子 Agent 配置 (参考 OpenClaw SpawnSubagentParams)
type SubAgentConfig struct {
	Label       string
	Task        string
	Instruction string
	Timeout     time.Duration
	Model       string
	Cleanup     string
}

// ResultStore 结果存储 (用于异步通知)
type ResultStore struct {
	mu       sync.RWMutex
	results  map[string]*SubAgentInfo
	filePath string
}

func NewResultStore(workspaceRoot string) *ResultStore {
	filePath := filepath.Join(workspaceRoot, ".subagent_results.jsonl")
	return &ResultStore{
		results:  make(map[string]*SubAgentInfo),
		filePath: filePath,
	}
}

// Save 保存结果到文件
func (r *ResultStore) Save(info *SubAgentInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results[info.SessionKey] = info

	// 追加到文件 (JSONL 格式)
	f, err := os.OpenFile(r.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		data, _ := json.Marshal(info)
		f.Write(data)
		f.WriteString("\n")
	}
}

// Get 获取结果
func (r *ResultStore) Get(sessionKey string) *SubAgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.results[sessionKey]
}

// GetAll 获取所有结果
func (r *ResultStore) GetAll() []*SubAgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*SubAgentInfo, 0, len(r.results))
	for _, info := range r.results {
		result = append(result, info)
	}
	return result
}

// SubAgentManager 子 Agent 管理器 (参考 OpenClaw subagent-registry)
type SubAgentManager struct {
	mu                sync.RWMutex
	sessions          map[string]*SubAgentInfo
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
	announceChan chan *Announce // 子 Agent 完成通知通道
}

// Announce 子 Agent 完成通知 (参考 OpenClaw)
type Announce struct {
	SessionKey string `json:"sessionKey"`
	Status     string `json:"status"` // completed/error
	Result     string `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
	Label      string `json:"label"`
	DurationMs int64  `json:"durationMs"`
}

// ClaudeRunner Claude Code CLI 进程管理器 (参考 OpenClaw 子进程执行)
type ClaudeRunner struct {
	workspace string
	sessionID string
	mode      string // "run" (one-shot) 或 "session" (persistent)
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.Reader
	stderr    io.Reader
	result    string
	isRunning bool
	isStopped bool
	mu        sync.Mutex

	// Session 模式专用
	taskChan   chan string   // 任务通道
	resultChan chan string   // 结果通道
	errChan    chan error    // 错误通道
	stopChan   chan struct{} // 停止信号通道
}

func NewSubAgentManager(cm model.ChatModel, workspaceRoot string, maxDepth int) *SubAgentManager {
	if maxDepth == 0 {
		maxDepth = 5
	}
	return &SubAgentManager{
		sessions:      make(map[string]*SubAgentInfo),
		cm:            cm,
		workspaceRoot: workspaceRoot,
		resultStore:   NewResultStore(workspaceRoot),
		depth:         0,
		maxDepth:      maxDepth,
		useClaudeCLI:  os.Getenv("USE_CLAUDE_CLI") == "true",
		claudeRunners: make(map[string]*ClaudeRunner), // Claude CLI 进程池
		announceChan:  make(chan *Announce, 10),       // Announce 通道
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
func (m *SubAgentManager) GetAnnounceChan() chan *Announce {
	return m.announceChan
}

// EmitAnnounce 发送 Announce 通知 (参考 OpenClaw runSubagentAnnounceFlow)
func (m *SubAgentManager) EmitAnnounce(announce *Announce) {
	select {
	case m.announceChan <- announce:
		fmt.Printf("│ 📬 [ANNOUNCE] Sent: session=%s, status=%s\n", announce.SessionKey, announce.Status)
	default:
		fmt.Printf("│ ⚠️  [ANNOUNCE] Channel full, dropping: session=%s\n", announce.SessionKey)
	}
}

// WaitForAnnounce 等待 Announce 通知 (阻塞等待)
func (m *SubAgentManager) WaitForAnnounce(timeout time.Duration) *Announce {
	select {
	case announce := <-m.announceChan:
		return announce
	case <-time.After(timeout):
		return nil
	}
}

// PollAnnounce 非阻塞获取 Announce
func (m *SubAgentManager) PollAnnounce() *Announce {
	select {
	case announce := <-m.announceChan:
		return announce
	default:
		return nil
	}
}

// NewClaudeRunner 创建 Claude Code CLI 运行器
// mode: "run" (one-shot, 每次任务新建进程) 或 "session" (persistent, 复用进程)
func NewClaudeRunner(workspace, sessionID, systemPrompt, task string, mode string) (*ClaudeRunner, error) {
	runner := &ClaudeRunner{
		workspace:  workspace,
		sessionID:  sessionID,
		mode:       mode,
		taskChan:   make(chan string, 1),
		resultChan: make(chan string, 1),
		errChan:    make(chan error, 1),
		stopChan:   make(chan struct{}),
	}

	// 构建命令
	// 使用 --print 非交互模式，--output-format json 获取结构化输出

	// 如果有 system prompt，添加到开头
	fullPrompt := ""
	if systemPrompt != "" {
		fullPrompt = systemPrompt + "\n\n"
	}
	fullPrompt += "Task: " + task

	// session 模式和 run 模式使用不同的方式
	var args []string
	if mode == "session" {
		// session 模式：不使用 -p，通过 stdin 发送任务，保持进程运行
		// 注意：需要确保 stdin 保持打开，进程才会等待输入
		args = []string{"--output-format", "stream-json", "--dangerously-skip-permissions", "--add-dir", workspace}
		fmt.Printf("│ 🆕 [CLAUDE] Session mode: stdin will be used (don't close after write)\n")
	} else {
		// run 模式：使用 -p，任务作为命令行参数
		args = []string{fullPrompt, "-p", "--output-format", "json", "--dangerously-skip-permissions", "--no-session-persistence", "--session-id", sessionID, "--add-dir", workspace}
		fmt.Printf("│ 🆕 [CLAUDE] Run mode: prompt length=%d\n", len(fullPrompt))
	}

	cmd := exec.Command("claude", args...)
	cmd.Dir = workspace

	// session 模式：将 stderr 重定向到 stdout 以便捕获所有输出
	// 或者使用我们自己的 writer 来调试
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	runner.stdin = stdin

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	runner.stdout = stdout

	runner.cmd = cmd
	fmt.Printf("│ 🆕 [CLAUDE] Runner created: workspace=%s, mode=%s, sessionID=%s\n", workspace, mode, sessionID)
	//fmt.Printf("│ 🆕 [CLAUDE] cmd.Dir=%s, cmd.Args=%v\n", workspace, args)
	return runner, nil
}

// Run 执行 Claude Code 子进程并返回结果
func (r *ClaudeRunner) Run(ctx context.Context) (string, error) {
	r.mu.Lock()
	r.isRunning = true
	r.mu.Unlock()

	fmt.Printf("│ 🔥 [ClaudeRunner] Starting process. \n")

	// 关键：先关闭 stdin，告诉 Claude CLI 输入已结束
	if r.stdin != nil {
		r.stdin.Close()
		r.stdin = nil
	}

	if err := r.cmd.Start(); err != nil {
		fmt.Printf("│ ❌ [ClaudeRunner] Failed to start: %v\n", err)
		return "", fmt.Errorf("failed to start claude: %w", err)
	}
	fmt.Printf("│ 🔥 [ClaudeRunner] Process started, PID=%d\n", r.cmd.Process.Pid)

	// 使用 bufio.Scanner 按行读取
	scanner := bufio.NewScanner(r.stdout)
	msgCount := 0
	for {
		select {
		case <-ctx.Done():
			r.cmd.Process.Kill()
			return "", ctx.Err()
		default:
		}

		// 设置扫描超时
		if !scanner.Scan() {
			err := scanner.Err()
			if err != nil {
				fmt.Printf("│ ❌ [ClaudeRunner] Scan error: %v\n", err)
			}
			fmt.Printf("│ 🔥 [ClaudeRunner] Scan done, msgCount=%d\n", msgCount)
			break
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		msgCount++
		fmt.Printf("│ 🔥 [ClaudeRunner] Msg #%d: %s\n", msgCount, line[:min(100, len(line))])

		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			fmt.Printf("│ ❌ [ClaudeRunner] JSON parse error: %v\n", err)
			continue
		}

		// 检查是否是最终结果
		if msg["type"] == "result" {
			fmt.Printf("│ 🔥 [ClaudeRunner] Got result: subtype=%v\n", msg["subtype"])
			if subtype, ok := msg["subtype"].(string); ok && subtype == "success" {
				if result, ok := msg["result"].(string); ok {
					r.result = result
					fmt.Printf("│ 🔥 [ClaudeRunner] Got success result, exiting...\n")
					// 不等待进程，直接返回
					r.cmd.Process.Kill()
					r.cmd.Wait()
					r.mu.Lock()
					r.isRunning = false
					r.mu.Unlock()
					return r.result, nil
				}
			}
			// 检查错误
			if isError, ok := msg["is_error"].(bool); ok && isError {
				fmt.Printf("│ ❌ [ClaudeRunner] Error result: %v\n", msg["result"])
				if errMsg, ok := msg["result"].(string); ok {
					return "", fmt.Errorf("claude error: %s", errMsg)
				}
				return "", fmt.Errorf("claude error: unknown")
			}
		}
	}

	// 等待进程结束
	r.cmd.Wait()

	r.mu.Lock()
	r.isRunning = false
	r.mu.Unlock()

	if r.result == "" {
		return "", fmt.Errorf("no result from claude")
	}

	return r.result, nil
}

// Start 启动 Claude 进程 (Session 模式)
func (r *ClaudeRunner) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cmd == nil {
		return fmt.Errorf("no command to start")
	}

	fmt.Printf("│ 🔥 [START] Calling cmd.Start()...\n")
	if err := r.cmd.Start(); err != nil {
		fmt.Printf("│ ❌ [START] cmd.Start() failed: %v\n", err)
		return err
	}

	r.isRunning = true
	r.isStopped = false
	fmt.Printf("│ 🔥 [START] Process running, PID=%d\n", r.cmd.Process.Pid)
	return nil
}

// listenOutputSession 监听输出 (Session 模式，持续监听)
func (r *ClaudeRunner) listenOutputSession() {
	scanner := bufio.NewScanner(r.stdout)
	for {
		if !scanner.Scan() {
			break
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		// 打印原始输出
		fmt.Printf("│ 📥 [STDOUT RAW] %s\n", line)

		fmt.Printf("│ 🔥 [SESSION] Output: %s\n", line[:min(100, len(line))])

		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			fmt.Printf("│ ❌ [SESSION] JSON parse error: %v\n", err)
			continue
		}

		// 检查是否是最终结果
		if msg["type"] == "result" {
			if subtype, ok := msg["subtype"].(string); ok && subtype == "success" {
				if result, ok := msg["result"].(string); ok {
					r.result = result
					fmt.Printf("│ 🔥 [SESSION] Received result: %s\n", result[:min(100, len(result))])
					r.resultChan <- result
				}
			}
			// 检查错误
			if isError, ok := msg["is_error"].(bool); ok && isError {
				errMsg := "unknown"
				if err, ok := msg["result"].(string); ok {
					errMsg = err
				}
				fmt.Printf("│ ❌ [SESSION] Received error: %s\n", errMsg)
				r.errChan <- fmt.Errorf("claude error: %s", errMsg)
			}
		}
	}
	fmt.Printf("│ 🔚 [SESSION] Output listener stopped\n")
}

// SendTask 发送任务 (Session 模式)
func (r *ClaudeRunner) SendTask(task string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stdin == nil {
		return fmt.Errorf("stdin is closed")
	}

	fmt.Printf("│ 📤 [SEND] Writing task to stdin (len=%d)...\n", len(task))
	n, err := fmt.Fprintf(r.stdin, "%s\n\n", task)
	fmt.Printf("│ 📤 [SEND] Wrote %d bytes\n", n)
	if err != nil {
		fmt.Printf("│ ❌ [SEND] Write failed: %v\n", err)
		return err
	}

	// 根据模式决定是否关闭 stdin
	if r.mode == "run" {
		// run 模式：关闭 stdin 发送 EOF 信号，告诉进程处理任务
		fmt.Printf("│ 📤 [SEND] Run mode: closing stdin to signal EOF...\n")
		r.stdin.Close()
		r.stdin = nil
	} else {
		// session 模式：保持 stdin 打开，进程会等待后续输入
		fmt.Printf("│ 📤 [SEND] Session mode: keeping stdin open for more tasks...\n")
	}

	return nil
}

// WaitResult 等待任务结果 (Session 模式)
func (r *ClaudeRunner) WaitResult(ctx context.Context) (string, error) {
	select {
	case result := <-r.resultChan:
		return result, nil
	case err := <-r.errChan:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// RunSession 运行任务 (Session 模式)
func (r *ClaudeRunner) RunSession(ctx context.Context, task string) (string, error) {
	// 检查进程是否已启动
	r.mu.Lock()
	isAlreadyRunning := r.isRunning && !r.isStopped
	r.mu.Unlock()

	if isAlreadyRunning {
		fmt.Printf("│ 🔄 [SESSION] Process already running, sending task...\n")
	} else {
		// 启动进程
		fmt.Printf("│ 🚀 [SESSION] Starting Claude process...\n")
		if err := r.Start(); err != nil {
			return "", fmt.Errorf("failed to start: %w", err)
		}
		fmt.Printf("│ 🔥 [SESSION] Claude process started, PID=%d\n", r.cmd.Process.Pid)

		// 启动输出监听协程（只在首次启动时）
		fmt.Printf("│ 🔄 [SESSION] Starting output listener...\n")
		go r.listenOutputSession()

		// 等待进程初始化
		fmt.Printf("│ ⏳ [SESSION] Waiting for process to initialize...\n")
		time.Sleep(500 * time.Millisecond)
	}

	// 发送任务
	fmt.Printf("│ 📤 [SESSION] Sending task: %s\n", task[:min(50, len(task))])
	if err := r.SendTask(task); err != nil {
		return "", fmt.Errorf("failed to send task: %w", err)
	}
	fmt.Printf("│ 📤 [SESSION] Task sent successfully\n")

	// 等待结果
	fmt.Printf("│ ⏳ [SESSION] Waiting for result...\n")
	return r.WaitResult(ctx)
}

// IsRunning 检查进程是否运行中
func (r *ClaudeRunner) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.isRunning && !r.isStopped
}

// Stop 停止 Claude Code 进程
func (r *ClaudeRunner) Stop() error {
	// 发送停止信号（如果是 session 模式）
	if r.mode == "session" && r.stopChan != nil {
		fmt.Printf("│ 🛑 [STOP] Sending stop signal (session mode)\n")
		select {
		case r.stopChan <- struct{}{}:
		default:
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.isStopped = true

	// 先关闭 stdin
	if r.stdin != nil {
		fmt.Printf("│ 🛑 [STOP] Closing stdin\n")
		r.stdin.Close()
		r.stdin = nil
	}

	if r.isRunning && r.cmd.Process != nil {
		fmt.Printf("│ 🛑 [STOP] Killing process PID=%d\n", r.cmd.Process.Pid)
		r.cmd.Process.Kill()
		r.cmd.Wait()
		r.isRunning = false
	}

	return nil
}

func generateSessionKey(label string) string {
	return fmt.Sprintf("subagent:%s:%s", label, uuid.New().String())
}

func generateUUID() string {
	return uuid.New().String()
}

// getFileExtension returns language name based on file extension
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
	info := &SubAgentInfo{
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

// recordInitialFiles 记录初始文件列表
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

// hashFiles 生成文件列表的哈希
func hashFiles(files []string) string {
	h := sha256.New()
	for _, f := range files {
		h.Write([]byte(f))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// SpawnAsyncInWorkspace 在指定工作区中创建子 Agent (共享 workspace)
func (m *SubAgentManager) SpawnAsyncInWorkspace(ctx context.Context, config *SubAgentConfig, existingWorkspacePath string) *SubAgentInfo {
	// 检查深度限制
	if m.depth >= m.maxDepth {
		errInfo := &SubAgentInfo{
			SessionKey: generateSessionKey(config.Label),
			Label:      config.Label,
			Status:     "error",
			Error:      fmt.Sprintf("max sub-agent depth (%d) exceeded", m.maxDepth),
		}
		m.EmitAnnounce(&Announce{
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
	info := &SubAgentInfo{
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
func (m *SubAgentManager) SpawnAsync(ctx context.Context, config *SubAgentConfig) *SubAgentInfo {
	// 检查深度限制
	if m.depth >= m.maxDepth {
		errInfo := &SubAgentInfo{
			SessionKey: generateSessionKey(config.Label),
			Label:      config.Label,
			Status:     "error",
			Error:      fmt.Sprintf("max sub-agent depth (%d) exceeded", m.maxDepth),
		}
		// 发送 Announce 通知错误
		m.EmitAnnounce(&Announce{
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
			errInfo := &SubAgentInfo{
				SessionKey: sessionKey,
				Label:      config.Label,
				Status:     "error",
				Error:      err.Error(),
			}
			// 发送 Announce 通知错误
			m.EmitAnnounce(&Announce{
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
	info := &SubAgentInfo{
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

// executeSubAgentAsync 后台异步执行子 Agent
func (m *SubAgentManager) executeSubAgentAsync(ctx context.Context, sessionKey string, config *SubAgentConfig, workspacePath string) {
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
	info := &SubAgentInfo{
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
	m.EmitAnnounce(&Announce{
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
func (m *SubAgentManager) executeWithChatModel(ctx context.Context, sessionKey string, config *SubAgentConfig, workspacePath string, startTime time.Time) string {
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
		m.resultStore.Save(&SubAgentInfo{
			SessionKey:    sessionKey,
			Label:         config.Label,
			Task:          config.Task,
			Status:        "error",
			Error:         err.Error(),
			DurationMs:    time.Since(startTime).Milliseconds(),
			WorkspacePath: workspacePath,
		})
		// 发送 Announce 通知错误
		m.EmitAnnounce(&Announce{
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
func (m *SubAgentManager) executeWithClaudeCLI(ctx context.Context, sessionKey string, config *SubAgentConfig, workspacePath string, startTime time.Time) string {
	fmt.Printf("│ 🔥 [CLAUDE CLI] Starting...\n")

	// 确定模式: run (one-shot) 或 session (persistent)
	// 通过环境变量 CLAUDE_SUBAGENT_MODE 设置，默认 run
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
		m.resultStore.Save(&SubAgentInfo{
			SessionKey:    sessionKey,
			Label:         config.Label,
			Task:          config.Task,
			Status:        "error",
			Error:         errMsg,
			DurationMs:    time.Since(startTime).Milliseconds(),
			WorkspacePath: workspacePath,
		})
		// 发送 Announce 通知错误
		m.EmitAnnounce(&Announce{
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
	//fmt.Printf("│ 🔥 [CLAUDE CLI] Task: %s\n", config.Task)

	if err != nil {
		errMsg := fmt.Sprintf("failed to get/create Claude runner: %v", err)
		fmt.Printf("│ ❌ [CLAUDE CLI] Failed to get runner: %v\n", err)
		m.updateResult(sessionKey, "error", "", errMsg, startTime)
		m.resultStore.Save(&SubAgentInfo{
			SessionKey:    sessionKey,
			Label:         config.Label,
			Task:          config.Task,
			Status:        "error",
			Error:         errMsg,
			DurationMs:    time.Since(startTime).Milliseconds(),
			WorkspacePath: workspacePath,
		})
		m.EmitAnnounce(&Announce{
			SessionKey: sessionKey,
			Status:     "error",
			Error:      errMsg,
			Label:      config.Label,
		})
		m.saveAndDecrementDepth(sessionKey, config.Cleanup, workspacePath)
		return ""
	}

	result := ""
	if mode == "session" {
		// Session 模式：发送任务并等待结果
		result, err = runner.RunSession(subCtx, config.Task)
	} else {
		// Run 模式：直接运行
		result, err = runner.Run(subCtx)
	}
	if err != nil {
		errMsg := fmt.Sprintf("claude cli error: %v", err)
		fmt.Printf("│ ❌ [CLAUDE CLI] Error: %v\n", err)
		m.updateResult(sessionKey, "error", "", errMsg, startTime)
		// 保存到 ResultStore
		m.resultStore.Save(&SubAgentInfo{
			SessionKey:    sessionKey,
			Label:         config.Label,
			Task:          config.Task,
			Status:        "error",
			Error:         errMsg,
			DurationMs:    time.Since(startTime).Milliseconds(),
			WorkspacePath: workspacePath,
		})
		// 发送 Announce 通知错误
		m.EmitAnnounce(&Announce{
			SessionKey: sessionKey,
			Status:     "error",
			Error:      errMsg,
			Label:      config.Label,
		})
		m.saveAndDecrementDepth(sessionKey, config.Cleanup, workspacePath)
		return ""
	}

	fmt.Printf("│ 🔥 [CLAUDE CLI] Result: %s\n", result[:min(200, len(result))])

	if result == "" {
		result = "No output from Claude CLI sub-agent"
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
		if status == "completed" {
			info.Result = result
		} else {
			info.Error = errMsg
		}
		now := time.Now()
		info.CompletedAt = &now
		info.DurationMs = time.Since(startTime).Milliseconds()
	}
}

func (m *SubAgentManager) saveAndDecrementDepth(sessionKey, cleanup, workspacePath string) {
	// 减少深度计数
	m.mu.Lock()
	m.depth--
	m.mu.Unlock()

	// 清理工作区
	if cleanup == "delete" {
		os.RemoveAll(workspacePath)
		fmt.Printf("🗑️  [CLEANUP] Workspace %s removed\n", workspacePath)
	}
}

// GetResult 获取子 Agent 结果 (供 get_subagent_result 工具调用)
func (m *SubAgentManager) GetResult(sessionKey string) *SubAgentInfo {
	// 先从内存获取
	m.mu.RLock()
	if info, ok := m.sessions[sessionKey]; ok {
		m.mu.RUnlock()
		return info
	}
	m.mu.RUnlock()

	// 从 ResultStore 获取
	return m.resultStore.Get(sessionKey)
}

// GetPendingAnnounces 获取待通知的结果 (推模式核心)
func (m *SubAgentManager) GetPendingAnnounces() []*SubAgentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*SubAgentInfo
	for _, info := range m.sessions {
		if info.Status == "completed" || info.Status == "error" {
			result = append(result, info)
		}
	}
	return result
}

// GetAnnounce 获取指定 session 的通知 (参考 OpenClaw GetAnnounce)
// 返回 Announce 结构，包含状态和结果
func (m *SubAgentManager) GetAnnounce(sessionKey string) *Announce {
	info := m.GetResult(sessionKey)
	if info == nil {
		return &Announce{
			SessionKey: sessionKey,
			Status:     "not_found",
			Error:      "session not found",
		}
	}

	announce := &Announce{
		SessionKey: info.SessionKey,
		Status:     info.Status,
	}

	if info.Status == "completed" {
		announce.Result = info.Result
	} else if info.Status == "error" {
		announce.Error = info.Error
	}

	return announce
}

// ListActiveSessions 列出活跃会话
func (m *SubAgentManager) ListActiveSessions() []*SubAgentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*SubAgentInfo, 0, len(m.sessions))
	for _, info := range m.sessions {
		result = append(result, info)
	}
	return result
}

// GetStats 获取统计
func (m *SubAgentManager) GetStats() (total, pending, running, completed, errors int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total = len(m.sessions)
	for _, info := range m.sessions {
		switch info.Status {
		case "pending":
			pending++
		case "running":
			running++
		case "completed":
			completed++
		case "error":
			errors++
		}
	}
	return
}

// ============================================================================
// 工具定义
// ============================================================================

// SpawnRequest spawn_subagent 工具请求
type SpawnRequest struct {
	Label       string `json:"label"`
	Task        string `json:"task"`
	Instruction string `json:"instruction,omitempty"`
	TimeoutSec  int    `json:"timeoutSec,omitempty"`
	Cleanup     string `json:"cleanup,omitempty"`
	Workspace   string `json:"workspace,omitempty"` // Existing workspace path to use
}

// SpawnResponse spawn_subagent 工具响应
type SpawnResponse struct {
	Status        string `json:"status"` // accepted, error
	SessionKey    string `json:"sessionKey"`
	Label         string `json:"label"`
	WorkspacePath string `json:"workspacePath"` // Workspace path for this sub-agent
	Note          string `json:"note"`
	Error         string `json:"error,omitempty"`
}

// GetResultRequest get_subagent_result 工具请求
type GetResultRequest struct {
	SessionKey string `json:"sessionKey"`
	Wait       bool   `json:"wait,omitempty"`
	WaitSec    int    `json:"waitSec,omitempty"`
}

// ListSessionsRequest list_subagent_sessions 工具请求 (参考 OpenClaw sessions_list)
type ListSessionsRequest struct {
	Status   string `json:"status,omitempty"`   // Filter by status: pending, running, completed, error, all
	Label    string `json:"label,omitempty"`    // Filter by label
	Limit    int    `json:"limit,omitempty"`    // Max results to return
	ActiveMs int    `json:"activeMs,omitempty"` // Filter by active minutes
}

// ListSessionsResponse list_subagent_sessions 工具响应 (参考 OpenClaw sessions_list)
type ListSessionsResponse struct {
	Sessions []*SubAgentInfo `json:"sessions"`
	Total    int             `json:"total"`
	Note     string          `json:"note"`
}

// GetResultResponse get_subagent_result 工具响应
type GetResultResponse struct {
	SessionKey    string `json:"sessionKey"`
	Status        string `json:"status"` // pending, running, completed, error
	Result        string `json:"result,omitempty"`
	Error         string `json:"error,omitempty"`
	DurationMs    int64  `json:"durationMs,omitempty"`
	WorkspacePath string `json:"workspacePath,omitempty"`
	Note          string `json:"note,omitempty"`
}

func buildSubAgentSystemPrompt(customInstruction, workspacePath string) string {
	base := fmt.Sprintf(`You are a Sub-Agent executing a specific task assigned by the Main Agent.

## Your Environment
- Working directory: %s
- You can read/write files in this directory
- All file operations should use absolute paths

## Your Role
- Work in an isolated session with a focused objective
- Complete the assigned task thoroughly
- Do NOT ask clarifying questions

## Output Format
1. **Accomplished**: What you completed
2. **Files Created**: List of files
3. **Issues**: Any problems
4. **Summary**: Brief summary`, workspacePath)

	if customInstruction != "" {
		base += "\n\n## Additional Instructions\n" + customInstruction
	}
	return base
}

func buildMainAgentSystemPrompt() string {
	return `You are a Main Agent that accomplishes complex tasks by spawning sub-agents.

## Your Capabilities
You can spawn sub-agents to handle complex tasks. Each sub-agent works in its own workspace.

## Sub-agent Workflow with Review (React Pattern)

### Step 1: Spawn
Use spawn_subagent to create a sub-agent. It returns IMMEDIATELY with a sessionKey.

### Step 2: Wait for Result (IMPORTANT!)
After spawning, you MUST immediately call get_subagent_result with:
- sessionKey: the key from spawn_subagent
- wait: true (REQUIRED - wait for completion)
- waitSec: 300 (max wait time)

The sub-agent is running in background. Use get_subagent_result to get its result.

### Step 3: Review Result
After getting the result, use analyze_code to analyze the code:
- analyze_code shares the same workspace with spawn_subagent
- DO NOT provide filePath - let the tool autonomously analyze
- DO NOT include file contents in your request
- analyze_code will automatically:
  1. Use git diff to detect changed files
  2. Read the files using Read tool
  3. Report any bugs, security issues, performance problems
- The analysis result will tell you:
  - Does the code compile?
  - Are there any bugs, security issues, or performance problems?
  - Does it meet the specifications?
  - canFix=true/false

### Step 4: Iterate if Needed (Optional)
If issues are found and canFix=true:
1. Use spawn_subagent with label like "fix-issues"
2. IMPORTANT: Use workspacePath from the first spawn_subagent result
   Example: workspace="/path/to/original/workspace" (NOT a new sessionKey)
3. In task, clearly describe what needs to be fixed (from analysis)
4. Use get_subagent_result to wait for the fix
5. Use analyze_code again to verify the fix (use original sessionKey)
6. Repeat until satisfied (max 3 iterations)

### Step 5: Final Response
Once satisfied with the result, present the final solution to the user.

## Critical Rules
1. ALWAYS call get_subagent_result AFTER spawn_subagent
2. ALWAYS use wait=true to wait for completion
3. DO NOT respond to user until you have the sub-agent result
4. Track session keys - you MUST use them to get results
5. DO NOT poll list_subagent_sessions in a loop - only check on-demand for debugging/intervention
6. Review the sub-agent result BEFORE responding to user
7. If issues found, spawn another sub-agent to fix them (max 3 iterations)

## Tools

### spawn_subagent
Spawn a sub-agent (async, returns immediately).
Parameters:
- label: Name for sub-agent (e.g., "code-writer", "fix-issues", "review-code")
- task: Task to accomplish (be specific!)
- instruction: Optional custom instructions
- timeoutSec: Timeout in seconds (default 600)
- cleanup: 'delete' or 'keep' workspace

Returns: sessionKey, status="accepted", note

### get_subagent_result
Get sub-agent result. MUST be called after spawn_subagent.
Parameters:
- sessionKey: The session key from spawn_subagent (REQUIRED)
- wait: true (REQUIRED - wait for completion)
- waitSec: 300 (max seconds to wait)

Returns: status, result, error, durationMs

### list_subagent_sessions (参考 OpenClaw sessions_list)
List sub-agent sessions with optional filters.
Parameters:
- status: Filter by status: pending, running, completed, error, all
- label: Filter by label
- limit: Max results (default 50)
- activeMs: Filter by active minutes

Returns: sessions array with sessionKey, status, label, durationMs
`

}

func buildMainAgentUserPrompt(task string) string {
	return fmt.Sprintf(`Complete this task using sub-agents:

Task: %s

Use spawn_subagent to create sub-agents. Check results with get_subagent_result.`, task)
}

// ============================================================================
// 主程序
// ============================================================================

func main() {
	// Gateway 模式
	if os.Getenv("GATEWAY_MODE") == "true" || len(os.Args) > 1 && os.Args[1] == "--gateway" {
		runGatewayMode()
		return
	}

	// CLI 模式
	var task string
	var timeout int
	var maxDepth int
	var workspace string
	var webhook string
	flag.StringVar(&task, "task", "", "the task")
	flag.IntVar(&timeout, "timeout", 600, "sub-agent timeout in seconds")
	flag.IntVar(&maxDepth, "max-depth", 5, "max nesting depth")
	flag.StringVar(&workspace, "workspace", "", "existing workspace directory (for developing on existing projects)")
	flag.StringVar(&webhook, "webhook", "", "webhook URL to send task completion notification")
	flag.Parse()

	// 验证 workspace 如果指定
	if workspace != "" {
		if _, err := os.Stat(workspace); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: workspace directory does not exist: %s\n", workspace)
			os.Exit(1)
		}
		fmt.Printf("📁 [WORKSPACE] Using existing workspace: %s\n", workspace)
	}

	// 验证 webhook 如果指定
	if webhook != "" {
		fmt.Printf("📡 [WEBHOOK] Will send notification to: %s\n", webhook)
	}

	if task == "" {
		task = `Create a Go HTTP server with:
1. /health endpoint
2. /api/hello endpoint returning JSON
3. Basic middleware for logging

Write complete code files.`
	}

	ctx := context.Background()
	startTime := time.Now()

	// 确定工作区根目录
	workspaceRoot := workspace
	if workspaceRoot == "" {
		workspaceRoot = "./workspace"
		os.MkdirAll(workspaceRoot, 0755)
	}

	cm, err := newChatModel(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create chat model: %v\n", err)
		os.Exit(1)
	}

	manager := NewSubAgentManager(cm, workspaceRoot, maxDepth)
	manager.SetExistingWorkspace(workspace) // 设置现有工作区

	// 创建 spawn_subagent 工具
	spawnTool := utils.NewTool(
		&schema.ToolInfo{
			Name: "spawn_subagent",
			Desc: "Spawn a sub-agent ASYNC (returns immediately). Use get_subagent_result to check completion.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"label":       {Type: "string", Desc: "Sub-agent label"},
				"task":        {Type: "string", Desc: "Task description"},
				"instruction": {Type: "string", Desc: "Custom instructions"},
				"timeoutSec":  {Type: "integer", Desc: "Timeout in seconds"},
				"cleanup":     {Type: "string", Desc: "Cleanup: delete/keep"},
				"workspace":   {Type: "string", Desc: "Existing workspace path to use (optional, for continuing work in same workspace)"},
			}),
		},
		func(ctx context.Context, input *SpawnRequest) (*SpawnResponse, error) {
			fmt.Println("════════════════════SpawnTool══════════════════════════")
			label := input.Label
			if label == "" {
				label = fmt.Sprintf("subagent-%d", time.Now().UnixMilli())
			}

			timeoutDur := time.Duration(input.TimeoutSec) * time.Second
			if timeoutDur == 0 {
				timeoutDur = time.Duration(timeout) * time.Second
			}

			cleanup := input.Cleanup
			if cleanup == "" {
				cleanup = "keep"
			}

			config := &SubAgentConfig{
				Label:       label,
				Task:        input.Task,
				Instruction: input.Instruction,
				Timeout:     timeoutDur,
				Cleanup:     cleanup,
			}

			// 异步 Spawn (立即返回)
			// 如果传递了 workspace 参数，使用已存在的 workspace
			var result *SubAgentInfo
			fmt.Printf("│ 📁 [SPAWN] Input workspace: '%s'\n", input.Workspace)
			if input.Workspace != "" {
				fmt.Printf("│ 📁 [SPAWN] Using EXISTING workspace: %s\n", input.Workspace)
				result = manager.SpawnAsyncInWorkspace(ctx, config, input.Workspace)
			} else {
				fmt.Printf("│ 📁 [SPAWN] Creating NEW workspace\n")
				result = manager.SpawnAsync(ctx, config)
			}

			return &SpawnResponse{
				Status:        result.Status,
				SessionKey:    result.SessionKey,
				Label:         result.Label,
				WorkspacePath: result.WorkspacePath,
				Note:          fmt.Sprintf("Sub-agent started. Use get_subagent_result with sessionKey='%s'. Workspace: %s", result.SessionKey, result.WorkspacePath),
				Error:         result.Error,
			}, nil
		},
	)

	// 创建 get_subagent_result 工具
	getResultTool := utils.NewTool(
		&schema.ToolInfo{
			Name: "get_subagent_result",
			Desc: "Get sub-agent result. MUST be called after spawn_subagent. Use wait=true to wait for completion.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"sessionKey": {Type: "string", Desc: "Session key from spawn_subagent (REQUIRED)"},
				"wait":       {Type: "boolean", Desc: "Wait for completion (default: true)"},
				"waitSec":    {Type: "integer", Desc: "Max wait seconds (default: 300)"},
			}),
		},
		func(ctx context.Context, input *GetResultRequest) (*GetResultResponse, error) {
			fmt.Println("════════════════════GetResultTool══════════════════════════")

			if input.SessionKey == "" {
				return &GetResultResponse{Error: "sessionKey required"}, nil
			}

			// 默认等待完成
			shouldWait := input.Wait
			if !shouldWait {
				// 默认等待完成
				shouldWait = true
			}

			// 等待完成
			if shouldWait {
				waitSec := input.WaitSec
				if waitSec == 0 {
					waitSec = 300 // 默认等待 5 分钟
				}
				waitDuration := time.Duration(waitSec) * time.Second
				deadline := time.Now().Add(waitDuration)

				// 快速路径：先检查是否已经完成
				info := manager.GetResult(input.SessionKey)
				if info != nil && (info.Status == "completed" || info.Status == "error") {
					return &GetResultResponse{
						SessionKey:    info.SessionKey,
						Status:        info.Status,
						Result:        info.Result,
						Error:         info.Error,
						DurationMs:    info.DurationMs,
						WorkspacePath: info.WorkspacePath,
						Note:          "Completed (immediate)",
					}, nil
				}

				// 使用 Announce 机制等待 (推模式，参考 OpenClaw)
				fmt.Printf("│ 📬 [WAIT] Using Announce mechanism, waiting for %ds...\n", waitSec)
				remainingTime := waitDuration
				for time.Now().Before(deadline) {
					// 使用更短的等待时间来响应中断
					checkInterval := 500 * time.Millisecond
					if remainingTime < checkInterval {
						checkInterval = remainingTime
					}

					// 使用 Announce 通道等待通知
					announce := manager.WaitForAnnounce(checkInterval)
					if announce != nil {
						// 收到通知，检查是否是目标 session
						if announce.SessionKey == input.SessionKey {
							fmt.Printf("│ 📬 [ANNOUNCE] Received for %s: status=%s\n", announce.SessionKey, announce.Status)
							// 从 resultStore 获取完整结果
							info := manager.GetResult(input.SessionKey)
							if info != nil {
								return &GetResultResponse{
									SessionKey:    info.SessionKey,
									Status:        info.Status,
									Result:        info.Result,
									Error:         info.Error,
									DurationMs:    info.DurationMs,
									WorkspacePath: info.WorkspacePath,
									Note:          "Completed via Announce",
								}, nil
							}
							// 如果 resultStore 中没有，使用 announce 中的信息
							return &GetResultResponse{
								SessionKey: announce.SessionKey,
								Status:     announce.Status,
								Result:     announce.Result,
								Error:      announce.Error,
								Note:       "Completed via Announce (resultStore not found)",
							}, nil
						}
						// 不是目标 session，继续等待
						fmt.Printf("│ 📬 [ANNOUNCE] Ignored other session: %s\n", announce.SessionKey)
					}
					remainingTime = deadline.Sub(time.Now())
				}
				fmt.Printf("│ ⏱️  [WAIT] Timeout waiting for result\n")
			}

			// 获取结果
			info := manager.GetResult(input.SessionKey)
			if info == nil {
				return &GetResultResponse{
					SessionKey: input.SessionKey,
					Status:     "not_found",
					Error:      "Session not found",
				}, nil
			}

			return &GetResultResponse{
				SessionKey:    info.SessionKey,
				Status:        info.Status,
				Result:        info.Result,
				Error:         info.Error,
				DurationMs:    info.DurationMs,
				WorkspacePath: info.WorkspacePath,
				Note:          fmt.Sprintf("Status: %s", info.Status),
			}, nil
		},
	)

	// 创建 list_subagent_sessions 工具 (参考 OpenClaw sessions_list)
	listSessionsTool := utils.NewTool(
		&schema.ToolInfo{
			Name: "list_subagent_sessions",
			Desc: "List sub-agent sessions with optional filters. (参考 OpenClaw sessions_list)",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"status":   {Type: "string", Desc: "Filter by status: pending, running, completed, error, all (default: all)"},
				"label":    {Type: "string", Desc: "Filter by label"},
				"limit":    {Type: "integer", Desc: "Max results to return (default: 50)"},
				"activeMs": {Type: "integer", Desc: "Filter by active minutes"},
			}),
		},
		func(ctx context.Context, input *ListSessionsRequest) (*ListSessionsResponse, error) {
			fmt.Println("════════════════════ListSessionsTool══════════════════════════")

			// 默认返回所有会话
			if input.Limit == 0 {
				input.Limit = 50
			}

			// 获取所有会话
			allSessions := manager.ListActiveSessions()

			// 过滤
			var filtered []*SubAgentInfo
			for _, s := range allSessions {
				// 按状态过滤
				if input.Status != "" && input.Status != "all" {
					if s.Status != input.Status {
						continue
					}
				}
				// 按标签过滤
				if input.Label != "" {
					if s.Label != input.Label {
						continue
					}
				}
				// 按活跃时间过滤
				if input.ActiveMs > 0 {
					activeMinutes := int(time.Since(s.CreatedAt).Minutes())
					if activeMinutes > input.ActiveMs {
						continue
					}
				}
				filtered = append(filtered, s)
				if len(filtered) >= input.Limit {
					break
				}
			}

			return &ListSessionsResponse{
				Sessions: filtered,
				Total:    len(filtered),
				Note:     fmt.Sprintf("Found %d sessions", len(filtered)),
			}, nil
		},
	)

	// 创建 read_workspace_file 工具 (用于 review 代码)
	readFileTool := utils.NewTool(
		&schema.ToolInfo{
			Name: "read_workspace_file",
			Desc: "Read a file from the workspace. Use this to review code written by sub-agents.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"sessionKey": {Type: "string", Desc: "Session key of the sub-agent workspace (REQUIRED)"},
				"filePath":   {Type: "string", Desc: "Relative file path to read (e.g., 'main.go', 'src/main.go')"},
			}),
		},
		func(ctx context.Context, input *struct {
			SessionKey string `json:"sessionKey"`
			FilePath   string `json:"filePath"`
		}) (*struct {
			Content    string `json:"content"`
			FilePath   string `json:"filePath"`
			SessionKey string `json:"sessionKey"`
			Error      string `json:"error,omitempty"`
		}, error) {
			fmt.Println("════════════════════ReadFileTool══════════════════════════")

			if input.SessionKey == "" {
				return &struct {
					Content    string `json:"content"`
					FilePath   string `json:"filePath"`
					SessionKey string `json:"sessionKey"`
					Error      string `json:"error,omitempty"`
				}{Error: "sessionKey required"}, nil
			}
			if input.FilePath == "" {
				return &struct {
					Content    string `json:"content"`
					FilePath   string `json:"filePath"`
					SessionKey string `json:"sessionKey"`
					Error      string `json:"error,omitempty"`
				}{Error: "filePath required"}, nil
			}

			// 获取工作区路径：优先使用 session 的 workspace，其次使用全局 existing workspace
			info := manager.GetResult(input.SessionKey)
			workspacePath := ""
			if info != nil && info.WorkspacePath != "" {
				workspacePath = info.WorkspacePath
			} else {
				workspacePath = manager.GetExistingWorkspace()
			}

			if workspacePath == "" {
				return &struct {
					Content    string `json:"content"`
					FilePath   string `json:"filePath"`
					SessionKey string `json:"sessionKey"`
					Error      string `json:"error,omitempty"`
				}{Error: "session not found or no workspace"}, nil
			}

			// 读取文件
			fullPath := filepath.Join(workspacePath, input.FilePath)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				return &struct {
					Content    string `json:"content"`
					FilePath   string `json:"filePath"`
					SessionKey string `json:"sessionKey"`
					Error      string `json:"error,omitempty"`
				}{Error: fmt.Sprintf("failed to read file: %v", err)}, nil
			}

			return &struct {
				Content    string `json:"content"`
				FilePath   string `json:"filePath"`
				SessionKey string `json:"sessionKey"`
				Error      string `json:"error,omitempty"`
			}{
				Content:    string(content),
				FilePath:   input.FilePath,
				SessionKey: input.SessionKey,
			}, nil
		},
	)

	// 创建 analyze_code 工具 (使用 subagent 进行代码分析)
	analyzeCodeTool := utils.NewTool(
		&schema.ToolInfo{
			Name: "analyze_code",
			Desc: "Analyze code for issues, bugs, and improvements using a sub-agent. Automatically analyzes all changes in the workspace using git diff.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"sessionKey": {Type: "string", Desc: "Session key of the sub-agent workspace to analyze (optional if using existing workspace)"},
				"focus":      {Type: "string", Desc: "Focus area: 'bugs', 'security', 'performance', 'style', 'all' (default: 'all')"},
			}),
		},
		func(ctx context.Context, input *struct {
			SessionKey string `json:"sessionKey"`
			Focus      string `json:"focus,omitempty"`
		}) (*struct {
			Analysis string `json:"analysis"`
			Issues   []struct {
				Severity string `json:"severity"`
				Type     string `json:"type"`
				Message  string `json:"message"`
				Line     int    `json:"line,omitempty"`
			} `json:"issues"`
			CanFix             bool   `json:"canFix"`
			Error              string `json:"error,omitempty"`
			AnalysisSessionKey string `json:"analysisSessionKey,omitempty"` // 用于跟踪分析会话
		}, error) {
			fmt.Println("════════════════════AnalyzeCodeTool══════════════════════════")

			// 使用 Claude Code CLI 子agent 自主分析（会自动使用 git diff/read 等工具）

			// 获取工作区路径
			var workspacePath string

			if input.SessionKey != "" {
				// 从 sessionKey 获取工作区
				info := manager.GetResult(input.SessionKey)
				if info != nil && info.WorkspacePath != "" {
					workspacePath = info.WorkspacePath
				}
			}

			// 如果没有 sessionKey 或无法从 sessionKey 获取，使用 existingWorkspace
			if workspacePath == "" {
				workspacePath = manager.GetExistingWorkspace()
			}

			if workspacePath == "" {
				return &struct {
					Analysis string `json:"analysis"`
					Issues   []struct {
						Severity string `json:"severity"`
						Type     string `json:"type"`
						Message  string `json:"message"`
						Line     int    `json:"line,omitempty"`
					} `json:"issues"`
					CanFix             bool   `json:"canFix"`
					Error              string `json:"error,omitempty"`
					AnalysisSessionKey string `json:"analysisSessionKey,omitempty"`
				}{Error: "no workspace available: provide sessionKey or set existing workspace"}, nil
			}

			fmt.Printf("│ 🔍 [ANALYZE] Using workspace: %s\n", workspacePath)

			// 使用 subagent 进行代码分析 - 始终使用 git diff 自主分析
			focus := input.Focus
			if focus == "" {
				focus = "all"
			}

			// 构建分析任务 - 让 Claude 自主使用 git diff/read 分析工作区改动
			analyzeTask := fmt.Sprintf(`你是一个专业的代码审查员。请分析工作区中的代码改动，识别问题并给出改进建议。

## 你的工作区
%s

## 分析重点
%s

## 任务
1. 首先使用 "git diff -- %s"  查看有哪些改动
2. 阅读工作区内的改动的文件,重要：只需要阅读在工作区目录下的改动的文件，其他父目录的文件忽略
3. 如果需要运行代码进行动态分析，请直接执行
4. 识别潜在的问题和改进点

## 请按照以下格式提供分析结果:

## 问题列表
对于每个问题，请提供:
- 严重程度: critical / warning / info
- 问题类型: bug / security / performance / style / other
- 问题描述: 问题的详细说明
- 文件和行号: 大致的文件和行号 (如果适用)

## 总体评估
- 代码是否可以编译运行?
- 存在哪些主要问题?
- 是否可以自动修复这些问题?

## 结论
这些代码问题是否可以自动修复? 请明确回答 "可以修复" 或 "无法自动修复"。`, workspacePath, focus, workspacePath)

			// 创建分析用的 subagent (共享 workspace)
			analysisLabel := "code-analyzer"
			analysisConfig := &SubAgentConfig{
				Label:       analysisLabel,
				Task:        analyzeTask,
				Instruction: "你是一个专业的代码审查员。请仔细分析代码，找出潜在的问题和改进点。如果需要运行代码进行动态分析，请直接执行。",
				Timeout:     5 * time.Minute,
				Cleanup:     "keep", // 保持工作区，因为是共享的
			}

			fmt.Printf("│ 🔍 [ANALYZE] Starting sub-agent for code analysis (workspace: %s)\n", workspacePath)

			// 使用 SpawnAsyncInWorkspace 共享工作区
			analysisInfo := manager.SpawnAsyncInWorkspace(ctx, analysisConfig, workspacePath)
			if analysisInfo.Status == "error" {
				return &struct {
					Analysis string `json:"analysis"`
					Issues   []struct {
						Severity string `json:"severity"`
						Type     string `json:"type"`
						Message  string `json:"message"`
						Line     int    `json:"line,omitempty"`
					} `json:"issues"`
					CanFix             bool   `json:"canFix"`
					Error              string `json:"error,omitempty"`
					AnalysisSessionKey string `json:"analysisSessionKey,omitempty"`
				}{Error: analysisInfo.Error}, nil
			}

			// 等待分析完成
			fmt.Printf("│ 🔍 [ANALYZE] Waiting for analysis to complete...\n")
			waitDuration := 5 * time.Minute
			deadline := time.Now().Add(waitDuration)
			var resultInfo *SubAgentInfo

			for time.Now().Before(deadline) {
				// 使用 Announce 等待
				announce := manager.WaitForAnnounce(2 * time.Second)
				if announce != nil && announce.SessionKey == analysisInfo.SessionKey {
					resultInfo = manager.GetResult(analysisInfo.SessionKey)
					break
				}
				// 检查状态
				resultInfo = manager.GetResult(analysisInfo.SessionKey)
				if resultInfo != nil && (resultInfo.Status == "completed" || resultInfo.Status == "error") {
					break
				}
			}

			if resultInfo == nil {
				return &struct {
					Analysis string `json:"analysis"`
					Issues   []struct {
						Severity string `json:"severity"`
						Type     string `json:"type"`
						Message  string `json:"message"`
						Line     int    `json:"line,omitempty"`
					} `json:"issues"`
					CanFix             bool   `json:"canFix"`
					Error              string `json:"error,omitempty"`
					AnalysisSessionKey string `json:"analysisSessionKey,omitempty"`
				}{Error: "analysis timeout"}, nil
			}

			// 解析分析结果
			analysisResult := resultInfo.Result
			if resultInfo.Status == "error" {
				analysisResult = resultInfo.Error
			}

			// 解析问题列表 (简化处理)
			var issues []struct {
				Severity string `json:"severity"`
				Type     string `json:"type"`
				Message  string `json:"message"`
				Line     int    `json:"line,omitempty"`
			}

			// 检查是否发现问题
			if strings.Contains(analysisResult, "critical") || strings.Contains(analysisResult, "warning") || strings.Contains(analysisResult, "bug") || strings.Contains(analysisResult, "问题") {
				issues = append(issues, struct {
					Severity string `json:"severity"`
					Type     string `json:"type"`
					Message  string `json:"message"`
					Line     int    `json:"line,omitempty"`
				}{
					Severity: "info",
					Type:     "review",
					Message:  "See analysis for details",
				})
			}

			// 判断是否可以自动修复
			canFix := strings.Contains(analysisResult, "可以修复") ||
				strings.Contains(analysisResult, "可以自动修复") ||
				strings.Contains(analysisResult, "can be fixed") ||
				strings.Contains(analysisResult, "true")

			canFix = canFix && (!strings.Contains(analysisResult, "无法自动修复") &&
				!strings.Contains(analysisResult, "cannot be fixed") &&
				!strings.Contains(analysisResult, "cannot fix"))

			fmt.Printf("│ 🔍 [ANALYZE] Analysis completed, canFix=%v\n", canFix)

			return &struct {
				Analysis string `json:"analysis"`
				Issues   []struct {
					Severity string `json:"severity"`
					Type     string `json:"type"`
					Message  string `json:"message"`
					Line     int    `json:"line,omitempty"`
				} `json:"issues"`
				CanFix             bool   `json:"canFix"`
				Error              string `json:"error,omitempty"`
				AnalysisSessionKey string `json:"analysisSessionKey,omitempty"`
			}{
				Analysis:           analysisResult,
				Issues:             issues,
				CanFix:             canFix,
				AnalysisSessionKey: analysisInfo.SessionKey,
			}, nil
		},
	)

	// 获取工具信息
	spawnInfo, _ := spawnTool.Info(ctx)
	getResultInfo, _ := getResultTool.Info(ctx)
	listSessionsInfo, _ := listSessionsTool.Info(ctx)
	readFileInfo, _ := readFileTool.Info(ctx)
	analyzeCodeInfo, _ := analyzeCodeTool.Info(ctx)

	// 绑定工具
	tcm, ok := cm.(model.ToolCallingChatModel)
	if !ok {
		fmt.Fprintln(os.Stderr, "model does not support tool calling")
		os.Exit(1)
	}

	// 定义对话状态 (用于累积历史消息)
	type chatState struct {
		history     []*schema.Message
		iterations  int  // 迭代次数限制
		subComplete bool // 子 Agent 是否已完成
	}

	tcmWithTools, err := tcm.WithTools([]*schema.ToolInfo{spawnInfo, getResultInfo, listSessionsInfo, readFileInfo, analyzeCodeInfo})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to bind tools: %v\n", err)
		os.Exit(1)
	}

	// 创建 Graph (带状态)
	g := compose.NewGraph[map[string]any, *schema.Message](
		compose.WithGenLocalState(func(ctx context.Context) *chatState {
			return &chatState{}
		}),
	)

	// 创建 ToolsNode
	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools: []tool.BaseTool{spawnTool, getResultTool, listSessionsTool, readFileTool, analyzeCodeTool},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create tools node: %v\n", err)
		os.Exit(1)
	}

	chatTpl := prompt.FromMessages(schema.FString,
		schema.SystemMessage(buildMainAgentSystemPrompt()),
		schema.MessagesPlaceholder("history", true),
		schema.UserMessage("{task}"),
	)

	_ = g.AddChatTemplateNode("template", chatTpl)

	// 添加 ChatModelNode (带状态处理)
	_ = g.AddChatModelNode("chat_model", tcmWithTools,
		compose.WithStatePreHandler(func(ctx context.Context, in []*schema.Message, state *chatState) ([]*schema.Message, error) {
			state.history = append(state.history, in...)
			return state.history, nil
		}),
		compose.WithStatePostHandler(func(ctx context.Context, out *schema.Message, state *chatState) (*schema.Message, error) {
			state.history = append(state.history, out)
			// 增加迭代计数，防止无限循环
			state.iterations++
			// 检查是否应该结束 (消息中没有 tool calls)
			if len(out.ToolCalls) == 0 {
				state.subComplete = true
			}
			return out, nil
		}),
	)

	// 添加 ToolsNode (带状态处理)
	_ = g.AddToolsNode("tools", toolsNode,
		compose.WithStatePreHandler(func(ctx context.Context, in *schema.Message, state *chatState) (*schema.Message, error) {
			// 从历史中获取最后一个消息（包含 tool call）
			if len(state.history) > 0 {
				return state.history[len(state.history)-1], nil
			}
			return in, nil
		}),
		compose.WithStatePostHandler(func(ctx context.Context, in []*schema.Message, state *chatState) ([]*schema.Message, error) {
			// 打印工具执行结果，方便调试主 agent 的思考过程
			if len(in) > 0 {
				fmt.Printf("│ 📥 [TOOL RESULT] Messages: %d\n", len(in))
			}
			return in, nil
		}),
	)

	_ = g.AddEdge(compose.START, "template")
	_ = g.AddEdge("template", "chat_model")
	_ = g.AddEdge("tools", "chat_model")

	// 使用 StreamGraphBranch 根据 tool_calls 决定流向
	// 同时检测迭代次数，防止无限循环
	var iterationCount int
	branch := compose.NewStreamGraphBranch(
		func(ctx context.Context, input *schema.StreamReader[*schema.Message]) (string, error) {
			defer input.Close()
			msg, err := input.Recv()
			if err != nil {
				return "", err
			}

			iterationCount++

			// 打印主 agent 的思考内容（如果有）
			if msg.Content != "" {
				contentStr := msg.Content
				if len(contentStr) > 300 {
					contentStr = contentStr[:300] + "..."
				}
				fmt.Printf("│ 💭 [ITERATION %d] Model思考: %s\n", iterationCount, contentStr)
			}

			fmt.Printf("📍 [ITERATION %d] ToolCalls: %d\n", iterationCount, len(msg.ToolCalls))

			// 超过最大迭代次数，强制结束
			if iterationCount >= 30 {
				fmt.Printf("⚠️  [MAX ITERATIONS] Reached limit, ending...\n")
				return compose.END, nil
			}

			if len(msg.ToolCalls) > 0 {
				// 打印工具调用信息
				for _, tc := range msg.ToolCalls {
					fmt.Printf("│ 🔧 [TOOL CALL] %s\n", tc.Function.Name)
				}
				return "tools", nil
			}

			fmt.Printf("│ ✅ [END] No more tool calls, ending graph\n")
			return compose.END, nil
		},
		map[string]bool{compose.END: true, "tools": true},
	)
	_ = g.AddBranch("chat_model", branch)

	graph, err := g.Compile(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to compile graph: %v\n", err)
		os.Exit(1)
	}

	// 打印头部
	fmt.Println("╔═══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║         LeeClaw Main/Sub Agent (Async Mode) Coding                            ║")
	fmt.Println("║                                                                               ║")
	fmt.Println("║  Async Features:                                                              ║")
	fmt.Println("║  • spawn_subagent: Returns immediately (async)                                ║")
	fmt.Println("║  • get_subagent_result: Poll for results                                      ║")
	fmt.Println("║  • Push-based notification via ResultStore                                    ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("📝 [Task] %s\n\n", task)

	// 运行
	result, err := graph.Invoke(ctx, map[string]any{
		"task":    buildMainAgentUserPrompt(task),
		"history": []*schema.Message{},
	})
	if err != nil {
		sendWebhook(webhook, task, manager, workspaceRoot, startTime)
		fmt.Fprintf(os.Stderr, "\n❌ Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                              Result")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	if result != nil && result.Content != "" {
		fmt.Println(result.Content)
	}

	printSummary(manager)

	// 停止所有 Claude CLI 进程 (session 模式)
	manager.StopAllClaudeRunners()

	// 发送 webhook 通知
	sendWebhook(webhook, task, manager, workspaceRoot, startTime)
}

// ============================================================================
// Gateway 模式
// ============================================================================

// runGatewayMode 运行 Gateway 服务器模式
func runGatewayMode() {
	// 解析命令行参数
	var port int
	var bind string
	var token string
	var password string
	var existingWorkspace string

	flag.IntVar(&port, "gateway-port", 18789, "gateway server port")
	flag.StringVar(&bind, "gateway-bind", "127.0.0.1", "gateway bind address")
	flag.StringVar(&token, "gateway-token", "", "gateway auth token")
	flag.StringVar(&password, "gateway-password", "", "gateway auth password")
	flag.StringVar(&existingWorkspace, "workspace", "", "existing workspace directory")
	flag.Parse()

	// 创建 SubAgentManager (传入 nil，因为 gateway 模式不需要模型)
	manager := NewSubAgentManager(nil, existingWorkspace, 5)

	// 创建 Gateway 配置
	config := &GatewayConfig{
		Port:     port,
		Bind:     bind,
		Token:    token,
		Password: password,
	}

	// 创建 Gateway
	gw := NewGateway(config, manager)

	// 启动 Gateway
	ctx := context.Background()
	if err := gw.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start gateway: %v\n", err)
		os.Exit(1)
	}

	// 等待退出信号
	fmt.Println("Press Ctrl+C to stop gateway")
	ch := make(chan os.Signal, 1)
	<-ch

	fmt.Println("Shutting down gateway...")
}

func printSummary(manager *SubAgentManager) {
	sessions := manager.ListActiveSessions()
	if len(sessions) == 0 {
		return
	}

	total, pending, running, completed, errors := manager.GetStats()

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                          Execution Summary")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	for _, s := range sessions {
		icon := "⚪"
		switch s.Status {
		case "pending":
			icon = "⏳"
		case "running":
			icon = "🔄"
		case "completed":
			icon = "✅"
		case "error":
			icon = "❌"
		}

		fmt.Printf("  %s %s\n", icon, s.SessionKey)
		fmt.Printf("     Label: %s | Status: %s | Duration: %dms\n", s.Label, s.Status, s.DurationMs)
		if s.Error != "" {
			fmt.Printf("     Error: %s\n", s.Error)
		} else if s.Result != "" {
			result := s.Result
			if len(result) > 100 {
				result = result[:100] + "..."
			}
			result = strings.ReplaceAll(result, "\n", " ")
			fmt.Printf("     Result: %s\n", result)
		}
		fmt.Println()
	}

	fmt.Printf("📈 Total: %d | Pending: %d | Running: %d | Completed: %d | Errors: %d\n",
		total, pending, running, completed, errors)
}

// TaskWebhookPayload 任务完成后的 webhook 负载
type TaskWebhookPayload struct {
	Task        string           `json:"task"`
	Status      string           `json:"status"` // success, partial, failed
	Summary     TaskSummary      `json:"summary"`
	Sessions    []SessionSummary `json:"sessions"`
	Workspace   string           `json:"workspace,omitempty"`
	DurationMs  int64            `json:"durationMs"`
	CompletedAt string           `json:"completedAt"`
}

// TaskSummary 任务摘要
type TaskSummary struct {
	Total     int `json:"total"`
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Errors    int `json:"errors"`
}

// SessionSummary 会话摘要
type SessionSummary struct {
	SessionKey    string `json:"sessionKey"`
	Label         string `json:"label"`
	Status        string `json:"status"`
	DurationMs    int64  `json:"durationMs"`
	ResultPreview string `json:"resultPreview,omitempty"`
}

// sendWebhook 发送 webhook 通知 (markdown 格式)
func sendWebhook(webhookURL string, task string, manager *SubAgentManager, workspace string, startTime time.Time) {
	if webhookURL == "" {
		return
	}

	sessions := manager.ListActiveSessions()
	total, _, running, completed, errors := manager.GetStats()

	// 构建 markdown 格式的消息
	var sessionDetails strings.Builder
	for _, s := range sessions {
		resultPreview := s.Result
		if len(resultPreview) > 100 {
			resultPreview = resultPreview[:100] + "..."
		}
		resultPreview = strings.ReplaceAll(resultPreview, "\n", " ")

		statusIcon := "⏳"
		if s.Status == "completed" {
			statusIcon = "✅"
		} else if s.Status == "error" {
			statusIcon = "❌"
		} else if s.Status == "running" {
			statusIcon = "🔄"
		}

		sessionDetails.WriteString(fmt.Sprintf("\n>- **%s** %s | %s | %dms",
			s.Label, statusIcon, s.Status, s.DurationMs))
		if resultPreview != "" {
			sessionDetails.WriteString(fmt.Sprintf("\n>  - 结果: %s", resultPreview))
		}
	}

	// 确定整体状态和图标
	statusIcon := "✅"
	status := "success"
	if errors > 0 {
		status = "partial"
		statusIcon = "⚠️"
	}
	if completed == 0 && errors > 0 {
		status = "failed"
		statusIcon = "❌"
	}

	// 构建 markdown 内容
	durationMs := time.Since(startTime).Milliseconds()
	markdownContent := fmt.Sprintf(`# 📊 任务执行完成

## 任务信息
- **任务**: %s
- **状态**: %s %s
- **工作区**: %s
- **耗时**: %dms
- **完成时间**: %s

## 执行统计
- **总计**: %d | **完成**: %d | **运行中**: %d | **错误**: %d

## 会话详情%s

---
> 由 LeeClaw 自动发送`,
		task,
		statusIcon,
		status,
		workspace,
		durationMs,
		time.Now().Format("2006-01-02 15:04:05"),
		total,
		completed,
		running,
		errors,
		sessionDetails.String(),
	)

	// 发送 webhook
	jsonData, err := json.Marshal(message{
		Type: "markdown",
		Markdown: messageContent{
			Content: markdownContent,
		},
	})
	if err != nil {
		fmt.Printf("❌ [WEBHOOK] Failed to marshal payload: %v\n", err)
		return
	}

	resp, err := defaultHttpClient.Post(webhookURL, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		fmt.Printf("❌ [WEBHOOK] Failed to send: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Printf("✅ [WEBHOOK] Notification sent successfully (status: %d)\n", resp.StatusCode)
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ [WEBHOOK] Failed with status %d: %s\n", resp.StatusCode, string(body))
	}
}

var (
	defaultHttpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: time.Second * 20,
	}
)

func qyNotify(content string, robotUrl string) {
	data, _ := json.Marshal(message{
		Type: "markdown",
		Markdown: messageContent{
			Content: content,
		},
	})
	resp, err := defaultHttpClient.Post(robotUrl, "application/json", bytes.NewReader(data))
	if err != nil {
		fmt.Printf("❌ [WEBHOOK] Failed to send: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Printf("✅ [WEBHOOK] Notification sent successfully (status: %d)\n", resp.StatusCode)
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ [WEBHOOK] Failed with status %d: %s\n", resp.StatusCode, string(body))
	}
	return
}

type message struct {
	Type     string         `json:"msgtype"`
	Markdown messageContent `json:"markdown"`
}

type messageContent struct {
	Content string `json:"content"`
}

func newChatModel(ctx context.Context) (model.ChatModel, error) {
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
