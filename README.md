# LeeClaw (OpenClaw-style) Main/Sub Agent

## 概述

本项目实现了如何基于 Eino 框架实现类似 OpenClaw 的主 Agent 与子 Agent 通讯机制。

## OpenClaw 核心机制

OpenClaw 是一个 AI 编码代理，其核心特点是主 Agent 可以 spawn（生成）多个子 Agent 来完成复杂任务：

1. **任务分解**: 主 Agent 分析复杂任务，将其分解为多个独立子任务
2. **Spawn 子 Agent**: 主 Agent 通过 spawn_subagent 工具创建子 Agent
3. **独立执行**: 每个子 Agent 在独立工作区中执行任务
4. **消息队列**: 子 Agent 完成后通过 announce 机制返回结果
5. **最终回复**: 主 Agent 汇总所有子 Agent 结果，给出最终回复

## 核心代码结构

```
Main Agent (Eino Graph)
    │
    ├── ChatTemplate (System Prompt)
    │
    ├── ChatModel (with Tools)
    │       │
    │       ├── spawn_subagent Tool (异步，立即返回 session_key)
    │       │       │
    │       │       └── SubAgentManager
    │       │           ├── sessions (会话管理)
    │       │           ├── pendingQueue (消息队列)
    │       │           ├── resultStore (结果持久化)
    │       │           ├── createWorkspace (工作区隔离)
    │       │           └── SpawnAsync() (异步创建)
    │       │
    │       └── get_subagent_result Tool (查询结果)
    │               │
    │               └── GetResult() / GetAnnounce()
    │
    └── ToolsNode (执行工具调用)
```

## 已实现功能

### 1. SubAgentManager (参考 OpenClaw subagent-registry)
- ✅ 会话管理 (`sessions` map)
- ✅ 消息队列 (`pendingQueue` - 参考 OpenClaw announce)
- ✅ `Spawn()` - 同步创建子 Agent (等待完成)
- ✅ `SpawnAsync()` - 异步创建子 Agent (立即返回)
- ✅ `EmitAnnounce()` - 发送子 Agent 完成通知 (推模式)
- ✅ `WaitForAnnounce()` - 阻塞等待 Announce 通知
- ✅ `PollAnnounce()` - 非阻塞获取 Announce 通知
- ✅ `GetAnnounce(sessionKey)` - 获取指定 session 的通知
- ✅ `GetResult()` - 获取子 Agent 执行结果
- ✅ 嵌套深度限制 (`maxDepth`)
- ✅ 工作区创建和隔离
- ✅ ResultStore - 结果持久化到 JSONL 文件

### 2. spawn_subagent 工具 (异步模式)
- ✅ 标签、任务、指令、超时时间
- ✅ 完整状态追踪 (pending → running → completed/error)
- ✅ 会话隔离
- ✅ 工作区清理选项 (delete/keep)
- ✅ 返回 session_key 用于后续查询
- ✅ SpawnAsync() 立即返回，不等待执行完成

### 3. get_subagent_result 工具 (参考 OpenClaw announce)
- ✅ 通过 session_key 查询子 Agent 状态
- ✅ 返回完整执行结果或当前状态
- ✅ 使用 WaitForAnnounce 推模式等待完成 (高效)
- ✅ 支持轮询模式作为后备
- ✅ GetAnnounce() 推模式通知完成

### 4. list_subagent_sessions 工具 (参考 OpenClaw sessions_list)
- ✅ 列出所有子 Agent 会话
- ✅ 按状态过滤 (pending/running/completed/error/all)
- ✅ 按标签过滤
- ✅ 按活跃时间过滤
- ✅ 返回会话列表和状态

### 5. read_workspace_file 工具
- ✅ 读取子 Agent 工作区中的文件
- ✅ 用于 review 代码内容

### 6. analyze_code 工具
- ✅ 使用 AI 分析代码问题
- ✅ 支持多种分析类型: bugs, security, performance, style
- ✅ 返回分析结果和可修复性判断
- ✅ 支持迭代修复流程

### 3. 工作区隔离
- ✅ 每个子 Agent 有独立工作区
- ✅ 工作区路径: `./workspace/subagent:{label}:{random}`
- ✅ 支持工作区清理

### 4. 生命周期管理
- ✅ 状态追踪: pending → running → completed/error
- ✅ 超时控制
- ✅ 深度限制 (防止无限嵌套)
- ✅ 执行统计

### 5. Claude Code CLI 模式 (可选)
- ✅ 使用 Claude Code CLI 作为子 Agent
- ✅ 独立进程执行，与主 Agent 完全隔离
- ✅ 支持完整 Claude Code 工具能力 (Bash, Edit, Read 等)
- ✅ 通过环境变量 `USE_CLAUDE_CLI=true` 启用
- ✅ **双模式支持**:
  - `run` 模式 (默认): 每次任务创建新进程，任务完成后退出
  - `session` 模式: 同一 workspace 复用进程，支持多轮对话
  - 通过环境变量 `CLAUDE_SUBAGENT_MODE=session` 启用 session 模式

### 6. 现有项目开发模式
- ✅ 支持指定现有工作区目录
- ✅ spawn_subagent 在现有目录中创建/修改文件
- ✅ analyze_code 直接分析现有代码
- ✅ 适用于功能扩展、Bug 修复等场景

## 运行

```bash
# 进入目录
cd quickstart/open-demo/cmd

# 设置环境变量
export OPENAI_API_KEY="your-key"
# 或使用豆包
export MODEL_TYPE="ark"
export ARK_API_KEY="your-key"

# 运行 (使用 API 模型作为子 Agent)
./open-demo --task "实现一个用户认证系统"

# 使用 Claude Code CLI 作为子 Agent (需要安装 claude CLI)
export USE_CLAUDE_CLI=true
./open-demo --task "实现一个用户认证系统"

# Claude CLI 子 Agent 模式配置
# run 模式 (默认): 每次任务创建新进程
export CLAUDE_SUBAGENT_MODE=run

# session 模式: 复用进程，支持多轮对话
export CLAUDE_SUBAGENT_MODE=session
./open-demo --task "实现一个用户认证系统"

# 自定义参数
./open-demo --task "..." --timeout 300 --max-depth 3

# 在现有项目上开发
./open-demo --workspace ./my-existing-project --task "添加用户认证功能"

# 使用 webhook 通知
./open-demo --task "实现一个 HTTP 服务器" --webhook "https://example.com/webhook"
```

## 参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| --task | 要完成的任务 | 无 |
| --timeout | 子 Agent 超时时间(秒) | 600 |
| --max-depth | 最大嵌套深度 | 5 |
| --workspace | 现有工作区目录 (用于在现有项目上开发) | 无 |
| --webhook | 任务完成后的 webhook URL | 无 |

## 输出示例

```
╔═══════════════════════════════════════════════════════════════════════════════╗
║          LeeClaw Main/Sub Agent Demo (Eino Framework)           ║
║                                                                               ║
║  Features:                                                                   ║
║  • Async sub-agent spawning with immediate return                          ║
║  • Session management with lifecycle tracking                               ║
║  • Automatic result collection via polling/GetAnnounce                      ║
║  • Result persistence to JSONL file                                         ║
║  • Workspace isolation and cleanup                                          ║
╚═══════════════════════════════════════════════════════════════════════════════╝

📝 [Task] 实现一个用户认证系统，包含注册、登录、权限管理...
⏱️  [Timeout] 600 seconds
📊 [Max Depth] 5

┌─────────────────────────────────────────────────────────────────────┐
│ 🤖 [SPAWN ASYNC] Sub-Agent: auth-module
│ 🔑 [SESSION] subagent:auth-module:a1b2c3d4
│ 📁 [WORKSPACE] ./workspace/subagent:auth-module:a1b2c3d4
│ ⏱️  [TIMEOUT] 10m0s
└─────────────────────────────────────────────────────────────────────┘
⏳ Sub-agent started, use session_key to query result...

# 主 Agent 可以继续处理其他任务，然后通过 get_subagent_result 查询

┌─────────────────────────────────────────────────────────────────────┐
│ 🔍 [QUERY] Getting result for: subagent:auth-module:a1b2c3d4
│ 📊 [STATUS] running... (still processing)
└─────────────────────────────────────────────────────────────────────┘

# 再次查询...

┌─────────────────────────────────────────────────────────────────────┐
│ 🔍 [QUERY] Getting result for: subagent:auth-module:a1b2c3d4
│ ✅ [STATUS] completed
│ ⏱️  [DURATION] 15000ms
└─────────────────────────────────────────────────────────────────────┘

✅ [COMPLETED] Sub-agent auth-module finished in 15000ms

═══════════════════════════════════════════════════════════════════════════════
                              Result
═══════════════════════════════════════════════════════════════════════════════

基于子 Agent 的结果，实现了以下用户认证系统...

═══════════════════════════════════════════════════════════════════════════════
                          Execution Summary
═══════════════════════════════════════════════════════════════════════════════

  ✅ subagent:auth-module:a1b2c3d4
     Label: auth-module | Status: completed | Duration: 15000ms
     Workspace: ./workspace/subagent:auth-module:a1b2c3d4
     Result: Implemented user authentication with...

📈 Total: 1 | Running: 0 | Completed: 1 | Errors: 0

💾 Results saved to: ./results/results.jsonl
```

## 与 OpenClaw 的对比

| 功能 | OpenClaw | 本实现 |
|------|----------|--------|
| 子 Agent Spawn | ✅ sessions_spawn | ✅ spawn_subagent |
| 异步 Spawn | ✅ | ✅ SpawnAsync() 立即返回 |
| 异步结果获取 | ✅ announce 推送 | ✅ get_subagent_result + WaitForAnnounce() |
| 会话列表 | ✅ sessions_list | ✅ list_subagent_sessions |
| 会话管理 | ✅ 独立进程/会话 | ✅ 内存会话 |
| 消息队列 | ✅ announce queue | ✅ pendingQueue + ResultStore |
| 结果持久化 | ✅ | ✅ JSONL 文件存储 |
| 工作区隔离 | ✅ 独立目录 | ✅ ./workspace/ |
| 嵌套深度限制 | ✅ | ✅ maxDepth |
| 工具清理选项 | ✅ | ✅ delete/keep |

## 核心类型

```go
// 子 Agent 信息
type SubAgentInfo struct {
    SessionKey     string     // 唯一会话 ID
    Label         string     // 标签
    Task          string     // 任务描述
    Status        string     // pending/running/completed/error
    Result        string     // 执行结果
    Error         string     // 错误信息
    CreatedAt     time.Time  // 创建时间
    CompletedAt   *time.Time // 完成时间
    DurationMs    int64      // 执行时间
    WorkspacePath string     // 工作区路径
    Model         string     // 使用的模型
}

// 子 Agent 配置
type SubAgentConfig struct {
    Label       string        // 标签
    Task        string        // 任务
    Instruction string        // 自定义指令
    Timeout     time.Duration // 超时时间
    Model       string        // 模型
    Cleanup     string        // 清理策略 (delete/keep)
}

// 子 Agent 管理器
type SubAgentManager struct {
    mu            sync.RWMutex
    sessions      map[string]*SubAgentInfo
    cm            model.ChatModel
    workspaceRoot string      // 工作区根目录
    resultStore   *ResultStore // 结果持久化
    depth         int          // 当前深度
    maxDepth      int          // 最大深度
}

// 结果存储 (用于异步结果持久化)
type ResultStore struct {
    mu      sync.RWMutex
    results map[string]*SubAgentInfo
    file    *os.File
}

// 异步通知 (参考 OpenClaw GetAnnounce)
type Announce struct {
    SessionKey string // 会话 ID
    Status     string // completed/error
    Result     string // 执行结果 (仅 completed 时)
    Error      string // 错误信息 (仅 error 时)
}
```

## 依赖

- github.com/cloudwego/eino - Eino 框架
- github.com/cloudwego/eino-ext/components/model/ark - 豆包模型
- github.com/cloudwego/eino-ext/components/model/openai - OpenAI 模型
- github.com/cloudwego/eino/components/tool/utils - 工具创建
- github.com/cloudwego/eino/compose - Graph 编排
