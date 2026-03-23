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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// ClaudeRunner Claude Code CLI 进程管理器 (参考 OpenClaw 子进程执行)
type ClaudeRunner struct {
	workspace  string
	sessionID  string
	mode       string // "run" (one-shot) 或 "session" (persistent)
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.Reader
	stderr     io.Reader
	result     string
	isRunning  bool
	isStopped  bool
	mu         sync.Mutex

	// Session 模式专用
	taskChan   chan string   // 任务通道
	resultChan chan string   // 结果通道
	errChan    chan error    // 错误通道
	stopChan   chan struct{} // 停止信号通道
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
