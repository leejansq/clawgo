/*
 * Chat Bot Management System - HTTP/WS Server
 * HTTP 服务器，处理 API 和 WebSocket
 */

package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/leejansq/clawgo/projects/chatbot/internal/chat"
	"github.com/leejansq/clawgo/projects/chatbot/internal/manager"
)

// Server HTTP 服务器
type Server struct {
	mgr         *manager.Manager
	chatHandler *chat.ChatHandler
	upgrader    websocket.Upgrader
	staticDir   string
	httpServer  *http.Server
}

// NewServer 创建服务器
func NewServer(mgr *manager.Manager, chatHandler *chat.ChatHandler, staticDir string) *Server {
	return &Server{
		mgr:         mgr,
		chatHandler: chatHandler,
		staticDir:   staticDir,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

// Start 启动服务器
func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	// 静态文件服务
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/chat/", s.handleChat)

	// WebSocket
	mux.HandleFunc("/ws/", s.handleWebSocket)

	// REST API
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/sessions/", s.handleSessionDetail)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Printf("🚀 Chat Bot Server starting on http://%s", addr)
	log.Printf("📝 Web Console: http://%s/chat/{sessionKey}", addr)

	return s.httpServer.ListenAndServe()
}

// handleIndex 首页
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// 显示欢迎页面和会话列表
	s.serveWelcomePage(w, r)
}

// serveWelcomePage 欢迎页面
func (s *Server) serveWelcomePage(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Chat Bot Management System</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
        h1 { color: #333; }
        .info { background: #f5f5f5; padding: 20px; border-radius: 8px; margin: 20px 0; }
        .sessions { margin-top: 20px; }
        .session-item { background: #fff; border: 1px solid #ddd; padding: 15px; margin: 10px 0; border-radius: 5px; }
        .session-item a { color: #007bff; text-decoration: none; }
        .session-item a:hover { text-decoration: underline; }
        .create-form { background: #e8f4ff; padding: 20px; border-radius: 8px; margin: 20px 0; }
        input, textarea { width: 100%; padding: 10px; margin: 5px 0; border: 1px solid #ddd; border-radius: 4px; box-sizing: border-box; }
        button { background: #007bff; color: white; padding: 10px 20px; border: none; border-radius: 4px; cursor: pointer; }
        button:hover { background: #0056b3; }
        .status { color: #666; font-size: 14px; }
    </style>
</head>
<body>
    <h1>🤖 Chat Bot Management System</h1>

    <div class="info">
        <h3>System Overview</h3>
        <p>This is a multi-agent chat system with a Main Agent managing multiple Sub-Agents.</p>
        <ul>
            <li><strong>Main Agent:</strong> Receives user instructions, spawns sub-agents</li>
            <li><strong>Sub-Agents:</strong> Independent chat instances with their own system prompts</li>
            <li><strong>Web Console:</strong> Each sub-agent has its own chat interface at /chat/{sessionKey}</li>
        </ul>
    </div>

    <div class="create-form">
        <h3>Create New Sub-Agent</h3>
        <form id="createForm">
            <input type="text" id="label" name="label" placeholder="Agent Label (e.g., Customer Service)" required>
            <textarea id="systemPrompt" name="systemPrompt" rows="3" placeholder="System Prompt (e.g., You are a helpful customer service agent)"></textarea>
            <button type="submit">Create Sub-Agent</button>
        </form>
        <div id="result" style="margin-top: 15px;"></div>
    </div>

    <div class="sessions">
        <h3>Active Sessions</h3>
        <div id="sessionsList">Loading...</div>
    </div>

    <script>
        // Create session
        document.getElementById('createForm').onsubmit = async (e) => {
            e.preventDefault();
            const label = document.getElementById('label').value;
            const systemPrompt = document.getElementById('systemPrompt').value;

            try {
                const res = await fetch('/api/sessions', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({label, systemPrompt})
                });
                const data = await res.json();
                if (data.sessionKey) {
                    document.getElementById('result').innerHTML =
                        '<p style="color:green;">✅ Created! <a href="' + data.url + '">Open Chat Console</a></p>';
                    loadSessions();
                } else {
                    document.getElementById('result').innerHTML = '<p style="color:red;">Error: ' + data.error + '</p>';
                }
            } catch (err) {
                document.getElementById('result').innerHTML = '<p style="color:red;">Error: ' + err.message + '</p>';
            }
        };

        // Load sessions
        async function loadSessions() {
            try {
                const res = await fetch('/api/sessions');
                const data = await res.json();
                const list = document.getElementById('sessionsList');
                if (data.sessions && data.sessions.length > 0) {
                    list.innerHTML = data.sessions.map(s => '<div class="session-item">' +
                        '<strong>' + s.label + '</strong> <span class="status">(' + s.status + ')</span><br>' +
                        '<small>Key: ' + s.sessionKey + '</small><br>' +
                        '<a href="/chat/' + encodeURIComponent(s.sessionKey) + '">Open Chat Console</a> | ' +
                        '<a href="/api/sessions/' + encodeURIComponent(s.sessionKey) + '/messages">View Messages</a>' +
                        '</div>').join('');
                } else {
                    list.innerHTML = '<p>No active sessions</p>';
                }
            } catch (err) {
                document.getElementById('sessionsList').innerHTML = '<p>Error loading sessions</p>';
            }
        }

        loadSessions();
        setInterval(loadSessions, 5000);
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// handleChat Web Console 页面
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	sessionKey := strings.TrimPrefix(r.URL.Path, "/chat/")
	if sessionKey == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	sessionKey, err := url.PathUnescape(sessionKey)
	if err != nil {
		http.Error(w, "Invalid session key", http.StatusBadRequest)
		return
	}

	// 检查会话是否存在
	chatSession, err := s.mgr.GetSession(sessionKey)
	if err != nil || chatSession == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// 读取 console.html 模板
	if s.staticDir != "" {
		consoleHTML := filepath.Join(s.staticDir, "console.html")
		data, err := os.ReadFile(consoleHTML)
		if err == nil {
			// 替换占位符
			html := strings.ReplaceAll(string(data), "{{SESSION_KEY}}", sessionKey)
			html = strings.ReplaceAll(html, "{{LABEL}}", chatSession.Label)

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(html))
			return
		}
	}

	// 如果文件不存在，返回嵌入式 HTML
	s.serveConsoleHTML(w, r, sessionKey, chatSession.Label)
}

// serveConsoleHTML 内嵌的 Console HTML
func (s *Server) serveConsoleHTML(w http.ResponseWriter, r *http.Request, sessionKey, label string) {
	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Chat - ` + label + `</title>
    <style>
        * { box-sizing: border-box; }
        body { font-family: Arial, sans-serif; margin: 0; padding: 0; background: #f5f5f5; }
        .header { background: #007bff; color: white; padding: 15px 20px; }
        .header h1 { margin: 0; font-size: 18px; }
        .header small { opacity: 0.8; }
        .container { max-width: 800px; margin: 0 auto; padding: 20px; }
        .messages { background: white; border-radius: 8px; height: 400px; overflow-y: auto; padding: 20px; margin-bottom: 20px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .message { margin-bottom: 15px; padding: 10px 15px; border-radius: 10px; max-width: 80%; }
        .message.user { background: #007bff; color: white; margin-left: auto; }
        .message.assistant { background: #e9ecef; color: #333; }
        .message .meta { font-size: 11px; opacity: 0.7; margin-top: 5px; }
        .input-area { display: flex; gap: 10px; }
        .input-area input { flex: 1; padding: 15px; border: 1px solid #ddd; border-radius: 25px; outline: none; }
        .input-area button { padding: 15px 30px; background: #007bff; color: white; border: none; border-radius: 25px; cursor: pointer; }
        .input-area button:hover { background: #0056b3; }
        .input-area button:disabled { background: #ccc; }
        .status { text-align: center; padding: 5px; font-size: 12px; color: #666; }
        .connected { color: green; }
        .disconnected { color: red; }
        pre { white-space: pre-wrap; word-wrap: break-word; }
    </style>
</head>
<body>
    <div class="header">
        <h1>💬 {{LABEL}}</h1>
        <small>Session: {{SESSION_KEY}}</small>
    </div>

    <div class="container">
        <div class="messages" id="messages"></div>
        <div class="input-area">
            <input type="text" id="messageInput" placeholder="Type your message..." onkeypress="if(event.key==='Enter')sendMessage()">
            <button onclick="sendMessage()" id="sendBtn">Send</button>
        </div>
        <div class="status" id="status">Connecting...</div>
    </div>

    <script>
        const sessionKey = "{{SESSION_KEY}}";
        const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
        const ws = new WebSocket(wsProtocol + '//' + location.host + '/ws/' + encodeURIComponent(sessionKey));
        const messagesDiv = document.getElementById('messages');
        const messageInput = document.getElementById('messageInput');
        const sendBtn = document.getElementById('sendBtn');
        const statusDiv = document.getElementById('status');

        ws.onopen = () => {
            statusDiv.textContent = 'Connected';
            statusDiv.className = 'status connected';
        };

        ws.onclose = () => {
            statusDiv.textContent = 'Disconnected - Refresh to reconnect';
            statusDiv.className = 'status disconnected';
            sendBtn.disabled = true;
        };

        ws.onerror = () => {
            statusDiv.textContent = 'Connection error';
            statusDiv.className = 'status disconnected';
        };

        ws.onmessage = (event) => {
            const data = JSON.parse(event.data);
            addMessage(data.role, data.content);
        };

        function addMessage(role, content) {
            const div = document.createElement('div');
            div.className = 'message ' + role;
            div.innerHTML = '<pre>' + escapeHtml(content) + '</pre>' +
                '<div class="meta">' + new Date().toLocaleTimeString() + '</div>';
            messagesDiv.appendChild(div);
            messagesDiv.scrollTop = messagesDiv.scrollHeight;
        }

        function sendMessage() {
            const text = messageInput.value.trim();
            if (!text) return;

            ws.send(JSON.stringify({message: text}));
            messageInput.value = '';
            addMessage('user', text);
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        // Load existing messages
        fetch('/api/sessions/' + encodeURIComponent(sessionKey) + '/messages')
            .then(r => r.json())
            .then(data => {
                if (data.messages) {
                    data.messages.forEach(m => addMessage(m.role, m.content));
                }
            });
    </script>
</body>
</html>`

	html = strings.ReplaceAll(html, "{{SESSION_KEY}}", sessionKey)
	html = strings.ReplaceAll(html, "{{LABEL}}", label)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// handleWebSocket 处理 WebSocket 连接
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionKey := strings.TrimPrefix(r.URL.Path, "/ws/")
	if sessionKey == "" {
		http.Error(w, "Session key required", http.StatusBadRequest)
		return
	}

	sessionKey, err := url.PathUnescape(sessionKey)
	if err != nil {
		http.Error(w, "Invalid session key", http.StatusBadRequest)
		return
	}

	// 检查会话是否存在
	chatSession, err := s.mgr.GetSession(sessionKey)
	if err != nil || chatSession == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// 升级为 WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// 处理消息
	s.handleWebSocketMessages(conn, sessionKey)
}

// handleWebSocketMessages 处理 WebSocket 消息
func (s *Server) handleWebSocketMessages(conn *websocket.Conn, sessionKey string) {
	ctx := context.Background()

	for {
		_, p, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// 解析消息
		var req struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(p, &req); err != nil {
			// 发送错误消息
			conn.WriteJSON(map[string]string{"error": "Invalid JSON"})
			continue
		}

		if req.Message == "" {
			continue
		}

		// 处理消息
		reply, err := s.chatHandler.HandleMessage(ctx, sessionKey, req.Message)
		if err != nil {
			conn.WriteJSON(map[string]string{"error": err.Error()})
			continue
		}

		// 发送回复
		conn.WriteJSON(map[string]string{
			"role":    "assistant",
			"content": reply,
		})
	}
}

// handleSessions 处理 /api/sessions
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// 列出所有会话
		sessions, err := s.mgr.ListSessions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 转换格式
		result := make([]map[string]interface{}, 0, len(sessions))
		for _, session := range sessions {
			result = append(result, map[string]interface{}{
				"sessionKey":   session.SessionKey,
				"label":        session.Label,
				"systemPrompt": session.SystemPrompt,
				"status":       session.GetStatus(),
				"createdAt":    session.CreatedAt.Format(time.RFC3339),
				"updatedAt":    session.UpdatedAt.Format(time.RFC3339),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"sessions": result,
			"total":    len(result),
		})

	case "POST":
		// 创建新会话
		var req struct {
			Label        string `json:"label"`
			SystemPrompt string `json:"systemPrompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Label == "" {
			req.Label = "Sub-Agent"
		}

		chatSession, sessionKey, err := s.mgr.CreateSession(req.Label, req.SystemPrompt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"sessionKey": sessionKey,
			"label":      chatSession.Label,
			"status":     chatSession.Status,
			"url":        "/chat/" + sessionKey,
			"createdAt":  chatSession.CreatedAt.Format(time.RFC3339),
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSessionDetail 处理 /api/session/{sessionKey}
func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/session/")

	// 匹配消息路由
	if strings.HasSuffix(path, "/messages") {
		sessionKey := strings.TrimSuffix(path, "/messages")
		sessionKey, err := url.PathUnescape(sessionKey)
		if err != nil {
			http.Error(w, "Invalid session key", http.StatusBadRequest)
			return
		}
		s.handleSessionMessages(w, r, sessionKey)
		return
	}

	sessionKey := path
	if sessionKey == "" {
		http.Error(w, "Session key required", http.StatusBadRequest)
		return
	}

	sessionKey, err := url.PathUnescape(sessionKey)
	if err != nil {
		http.Error(w, "Invalid session key", http.StatusBadRequest)
		return
	}

	// 获取或删除会话
	switch r.Method {
	case "GET":
		chatSession, err := s.mgr.GetSession(sessionKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if chatSession == nil {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"sessionKey":   chatSession.SessionKey,
			"label":        chatSession.Label,
			"systemPrompt": chatSession.SystemPrompt,
			"status":       chatSession.GetStatus(),
			"createdAt":    chatSession.CreatedAt.Format(time.RFC3339),
			"updatedAt":    chatSession.UpdatedAt.Format(time.RFC3339),
		})

	case "DELETE":
		if err := s.mgr.DeleteSession(sessionKey); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSessionMessages 处理 /api/sessions/{sessionKey}/messages
func (s *Server) handleSessionMessages(w http.ResponseWriter, r *http.Request, sessionKey string) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	messages, err := s.mgr.GetMessages(sessionKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 转换格式
	result := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		result = append(result, map[string]interface{}{
			"role":      msg.Role,
			"content":   msg.Content,
			"createdAt": msg.CreatedAt.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"messages": result,
		"total":    len(result),
	})
}
