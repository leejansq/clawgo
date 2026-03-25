# Dual-Layer Memory System

基于 OpenClaw 的双层记忆机制实现的 Go 版本。

## 目录结构

```
pkg/memory/
├── types.go           # 类型定义
├── store.go           # 核心接口 MemoryStore
├── implementation.go  # 主要实现
├── vector.go          # 向量存储（SQLite + BM25）
├── long_term.go       # 长期记忆实现
├── short_term.go      # 短期记忆实现
├── hybrid.go          # 混合搜索 + MMR + 时间衰减
├── tools.go           # Agent Tool 实现
├── search.go          # 搜索辅助函数
└── demo/
    └── main.go        # Demo 程序
```

## 核心概念

### 记忆类型

| 类型 | 文件位置 | 说明 |
|------|---------|------|
| 长期记忆 | `MEMORY.md` | 持久化存储的重要信息，不受时间衰减影响 |
| 短期记忆 | `memory/YYYY-MM-DD.md` | 每日会话日志，受时间衰减影响 |

### Agent Tool

#### memory_search

语义搜索工具，**Agent 必须先调用此工具才能回答关于过去工作、决定、日期、人员、偏好或待办事项的问题**。

```go
// Tool 描述（类似 OpenClaw）
name: "memory_search"
description: "Mandatory recall step: semantically search MEMORY.md + memory/*.md
before answering questions about prior work, decisions, dates, people,
preferences, or todos; returns top snippets with path + lines."
```

#### memory_get

安全读取工具，**在 memory_search 之后使用，用于获取具体行内容**。

```go
// Tool 描述
name: "memory_get"
description: "Safe snippet read from MEMORY.md or memory/*.md with
optional from/lines; use after memory_search to pull only the needed
lines and keep context small."
```

## 使用示例

### 1. 创建 MemoryStore

```go
store, err := memory.New(ctx, &memory.Config{
    BaseDir:      "/tmp/eino/memory",
    VectorDBPath: "/tmp/eino/memory/vector.db",
    Embedder:     embedder,  // 可选，无 embedder 则只用 BM25
    ChunkSize:    512,
    ChunkOverlap: 50,
})
```

### 2. 写入记忆

```go
// 长期记忆
store.WriteLongTerm(ctx, "# 用户偏好\n- 喜欢 Go 语言\n- 喜欢简洁代码")

// 短期记忆
store.Write(ctx, "今天的会议讨论了 OAuth2 实现", memory.MemoryMeta{
    Type:   memory.MemoryTypeShortTerm,
    Date:   "2026-03-24",
    Source: "meeting",
    Tags:   []string{"feature", "auth"},
})
```

### 3. 使用 Tool（推荐方式）

```go
// 配置（类似 OpenClaw 默认配置）
cfg := memory.DefaultMemoryToolsConfig()
// 或自定义配置:
cfg := &memory.MemoryToolsConfig{
    EnableHybridSearch: true,
    VectorWeight:       0.7,
    EnableMMR:          false,  // 显式开启
    EnableTimeDecay:    false,  // 显式开启
}

// memory_search
result, _ := memory.MemorySearchTool(ctx, store, memory.MemorySearchParams{
    Query: "OAuth2 JWT authentication",
}, cfg)

// memory_get
result, _ := memory.MemoryGetTool(ctx, store, memory.MemoryGetParams{
    Path:  "memory/2026-03-24.md",
    From:  intPtr(1),
    Lines: intPtr(10),
})
```

### 4. 搜索选项

```go
results, err := store.Search(ctx, "authentication",
    memory.WithSearchLimit(5),
    memory.WithSearchHybrid(true),
    memory.WithSearchVectorWeight(0.7),
    memory.WithSearchMMR(true),
    memory.WithSearchMMRLambda(0.5),
    memory.WithSearchTimeDecay(true),
    memory.WithSearchTimeDecayFactor(0.95),
    memory.WithSearchMemoryTypes(memory.MemoryTypeShortTerm),
    memory.WithSearchDates("2026-03-24"),
)
```

## 搜索技术

### 混合搜索

```
用户查询 → 向量搜索 + BM25 关键词搜索 → 加权合并 → 时间衰减 → MMR 重排 → 结果
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `hybrid.enabled` | true | 启用混合搜索 |
| `hybrid.vectorWeight` | 0.7 | 向量搜索权重 |
| `hybrid.textWeight` | 0.3 | BM25 权重 |

### MMR 重排序

平衡相关性与多样性：

```go
MMR = λ * 相关度 - (1-λ) * 最大相似度

// λ = 1: 只看相关度
// λ = 0.7: 平衡（推荐）
// λ = 0: 只看多样性
```

### 时间衰减

基于半衰期的指数衰减：

```go
score = score * e^(-λ * age_days)
// 半衰期 30 天 = 30 天后分数衰减为原来的一半
```

**不受衰减影响的记忆：**
- `MEMORY.md` (长期)
- `memory/topic.md` (主题)

**受衰减影响的记忆：**
- `memory/YYYY-MM-DD.md` (每日)
- 会话记录

## 配置对比

| 配置项 | OpenClaw 默认 | 本实现默认 | 说明 |
|--------|--------------|-----------|------|
| `hybrid.enabled` | true | true | 混合搜索 |
| `vectorWeight` | 0.7 | 0.7 | 向量权重 |
| `mmr.enabled` | **false** | **false** | 需显式开启 |
| `mmr.lambda` | 0.7 | 0.7 | MMR 参数 |
| `temporalDecay.enabled` | **false** | **false** | 需显式开启 |
| `temporalDecay.halfLifeDays` | 30 | 30 | 半衰期 |

## 运行 Demo

```bash
cd quickstart/eino_assistant
go run ./pkg/memory/demo/main.go
```
