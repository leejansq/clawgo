# Chat Bot Management System

A multi-agent chat system with a Main Agent managing multiple Sub-Agents, inspired by OpenClaw's architecture.

## Features

- **Main Agent (Manager)**: Receives user instructions, spawns sub-agents
- **Sub-Agents**: Independent chat instances with their own system prompts and contexts
- **Web Console**: Each sub-agent has its own chat interface at `/chat/{sessionKey}`
- **WebSocket Communication**: Real-time chat via WebSocket
- **SQLite Storage**: Persists chat records and session metadata
- **REST API**: Programmatic access to session management

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Main Agent                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│  │   Manager   │  │  ChatHandler │  │   Server   │          │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘          │
│         │                 │                 │                │
│  ┌──────▼─────────────────▼─────────────────▼──────┐       │
│  │                   SQLite Store                    │       │
│  └───────────────────────────────────────────────────┘       │
└─────────────────────────────────────────────────────────────┘
                              │
          ┌───────────────────┼───────────────────┐
          │                   │                   │
    ┌─────▼─────┐       ┌─────▼─────┐       ┌─────▼─────┐
    │ Sub-Agent │       │ Sub-Agent │       │ Sub-Agent │
    │  (chat:1)  │       │  (chat:2)  │       │  (chat:3)  │
    └───────────┘       └───────────┘       └───────────┘
```

## Quick Start

### 1. Install Dependencies

```bash
cd projects/chatbot
go mod tidy
```

### 2. Run the Server

```bash
go run cmd/main.go
```

The server will start on `http://127.0.0.1:18888`

### 3. Access the System

- **Home Page**: http://127.0.0.1:18888/
- **Create Sub-Agent**: Use the web form or API
- **Web Console**: http://127.0.0.1:18888/chat/{sessionKey}

## API Reference

### Create Sub-Agent

```bash
curl -X POST http://127.0.0.1:18888/api/sessions \
  -H "Content-Type: application/json" \
  -d '{
    "label": "Customer Service",
    "systemPrompt": "You are a helpful customer service agent that helps users with refund issues."
  }'
```

Response:
```json
{
  "sessionKey": "chatbot:Customer Service:uuid",
  "label": "Customer Service",
  "status": "active",
  "url": "/chat/chatbot:Customer Service:uuid",
  "createdAt": "2026-03-25T10:00:00Z"
}
```

### List All Sessions

```bash
curl http://127.0.0.1:18888/api/sessions
```

### Get Session Info

```bash
curl http://127.0.0.1:18888/api/session/chatbot:Customer%20Service:uuid
```

### Get Session Messages

```bash
curl http://127.0.0.1:18888/api/sessions/chatbot:Customer%20Service:uuid/messages
```

### Delete Session

```bash
curl -X DELETE http://127.0.0.1:18888/api/session/chatbot:Customer%20Service:uuid
```

### WebSocket Chat

Connect to: `ws://127.0.0.1:18888/ws/chatbot:Customer%20Service:uuid`

Send message:
```json
{"message": "Hello, I need help with a refund"}
```

Receive reply:
```json
{"role": "assistant", "content": "Hello! I'd be happy to help you with your refund request..."}
```

## Project Structure

```
projects/chatbot/
├── cmd/
│   └── main.go              # Entry point
├── internal/
│   ├── server/
│   │   └── server.go        # HTTP/WS server
│   ├── manager/
│   │   └── manager.go       # Main agent manager
│   ├── session/
│   │   └── session.go       # Chat session
│   ├── store/
│   │   └── sqlite.go        # SQLite storage
│   └── chat/
│       └── chat.go          # Chat logic with LLM
├── static/
│   └── console.html         # Web Console page
├── go.mod
└── README.md
```

## Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `127.0.0.1:18888` | Server address |
| `-db` | `./chatbot.db` | SQLite database path |
| `-static` | `` | Static files directory |

## Database Schema

### sessions 表

| Column | Type | Description |
|--------|------|-------------|
| id | INTEGER | Primary key |
| session_key | TEXT | Unique session ID |
| label | TEXT | Human-readable label |
| system_prompt | TEXT | System prompt for the sub-agent |
| status | TEXT | Session status (active/closed) |
| created_at | DATETIME | Creation timestamp |
| updated_at | DATETIME | Last update timestamp |

### messages 表

| Column | Type | Description |
|--------|------|-------------|
| id | INTEGER | Primary key |
| session_key | TEXT | Foreign key to sessions |
| role | TEXT | Message role (user/assistant) |
| content | TEXT | Message content |
| created_at | DATETIME | Creation timestamp |

## LLM Integration

The system currently supports a mock response mode. To enable real LLM responses, configure the LLM in `internal/chat/chat.go`:

```go
// Initialize with LLM model
cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
    APIKey: os.Getenv("ARK_API_KEY"),
    Model:  "doubao-pro",
})
chatHandler := chat.NewChatHandler(mgr, cm)
```

## License

MIT
