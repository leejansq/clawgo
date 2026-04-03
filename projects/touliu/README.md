# 电商投流多 Agent 系统

基于 clawgo 框架开发的多 Agent 协作系统，用于电商广告投放的智能化运营。

## 系统架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                     Coordinator (协调器)                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐         │
│  │  Agent-A     │───▶│  Agent-B    │───▶│  Agent-C     │         │
│  │  市场调研     │    │  投放执行    │    │  效果评估    │         │
│  │  策略师      │    │  执行员      │    │  经验沉淀    │         │
│  └──────────────┘    └──────────────┘    └──────────────┘         │
│         │                                                         │
│         ▼                                                         │
│  ┌──────────────────────────────────────────────────────────────┐ │
│  │  ⏸️ Human-in-Loop 确认节点                                    │ │
│  │     用户确认方案后方可执行                                      │ │
│  └──────────────────────────────────────────────────────────────┘ │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐ │
│  │  AdAgent Base (LLM + Session + Memory + Skills)              │ │
│  │   - LLM: eino ChatModel (GPT/Claude)                         │ │
│  │   - Session: 上下文追踪 (session.SessionStore)                │ │
│  │   - Memory: 经验存储 (memory.MemoryStore)                     │ │
│  │   - Skills: 工具扩展 (skill.SkillLoader)                       │ │
│  └──────────────────────────────────────────────────────────────┘ │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

## 核心特性

### Human-in-Loop 确认
Agent-A 生成投流方案后，系统会暂停等待用户确认：
- **y/yes** - 确认执行
- **n/no** - 拒绝并退出
- **c/change** - 修改方案（输入修改说明后重新生成）

### 多 Agent 协作

| Agent | 职责 | Session | Memory |
|-------|------|---------|--------|
| **Agent-A** | 市场调研与策略师 | 分析对话历史 | 存储投放方案 |
| **Agent-B** | 投放执行员 | 记录执行过程 | 存储执行结果 |
| **Agent-C** | 效果评估与经验沉淀 | 评估对话记录 | 存储经验教训 |

## 快速开始

### 1. 设置环境变量

```bash
export OPENAI_API_KEY="your-api-key"
export OPENAI_MODEL="gpt-4o"
```

### 2. 运行系统

```bash
go run ./projects/touliu/cmd/main.go \
  -product "智能手表" \
  -platform "douyin" \
  -market "一线城市"
```

### 3. 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-product` | 智能手表 | 产品名称 |
| `-platform` | douyin | 投放平台: douyin/weibo/toutiao |
| `-market` | 一线城市 | 目标市场 |
| `-knowledge` | ./knowledge | 知识库路径 |
| `-workspace` | ./workspace | 工作空间路径 |
| `-skills` | ./skills | 技能目录 |

## 项目结构

```
projects/touliu/
├── cmd/
│   └── main.go              # 主入口
├── config/
│   └── config.yaml          # 配置文件
├── internal/
│   ├── agents/
│   │   ├── ad_agent.go     # Agent 基础类 (LLM+Session+Memory+Skills)
│   │   ├── agent_a.go       # 市场调研策略师
│   │   ├── agent_b.go       # 投放执行员
│   │   ├── agent_c.go       # 效果评估员
│   │   ├── coordinator.go   # 工作流协调器 (含 Human-in-Loop)
│   │   ├── workflow.go      # 工作流上下文
│   │   ├── types.go        # 类型定义
│   │   └── utils.go        # 工具函数
│   └── types/
│       └── types.go         # 核心类型定义
└── README.md
```

## 内置工具

每个 Agent 都内置以下工具：

| 工具 | 功能 |
|------|------|
| `memory_read` | 从记忆库搜索历史经验和方案 |
| `memory_write` | 将重要信息写入记忆库 |
| `session_append` | 追加消息到会话上下文 |

## 执行流程

```
1. Agent-A 分析市场数据，生成投放方案
   ↓
2. ⏸️ Human-in-Loop: 用户确认方案 (y/n/c)
   ↓ (确认后)
3. Agent-B 执行投放操作
   ↓
4. Agent-C 评估效果，沉淀经验
   ↓
5. 完成
```

## 扩展开发

### 添加自定义技能

在 `./skills` 目录下创建 `SKILL.md` 文件：

```markdown
---
name: custom_tool
description: 自定义工具描述
user-invocable: true
priority: 100
---

# Custom Tool

工具使用说明...
```

## License

Apache 2.0
