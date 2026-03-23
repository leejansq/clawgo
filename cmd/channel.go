package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// Channel 系统
// ============================================================================

// ChannelType 渠道类型
type ChannelType string

const (
	ChannelTypeWebhook ChannelType = "webhook"
	ChannelTypeTelegram ChannelType = "telegram"
	ChannelTypeSlack   ChannelType = "slack"
)

// Channel 消息渠道接口
type Channel interface {
	// GetType 获取渠道类型
	GetType() ChannelType
	// GetID 获取渠道 ID
	GetID() string
	// Start 启动渠道服务
	Start(ctx context.Context) error
	// Stop 停止渠道服务
	Stop() error
	// SendMessage 发送消息
	SendMessage(to, content string) error
	// HandleWebhook 处理 Webhook 回调
	HandleWebhook(w http.ResponseWriter, r *http.Request)
}

// ChannelManager 渠道管理器
type ChannelManager struct {
	channels map[string]Channel
	mu       sync.RWMutex
}

// NewChannelManager 创建渠道管理器
func NewChannelManager() *ChannelManager {
	return &ChannelManager{
		channels: make(map[string]Channel),
	}
}

// Register 注册渠道
func (m *ChannelManager) Register(channel Channel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels[channel.GetID()] = channel
}

// Get 获取渠道
func (m *ChannelManager) Get(id string) Channel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.channels[id]
}

// List 列出所有渠道
func (m *ChannelManager) List() []Channel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		result = append(result, ch)
	}
	return result
}

// StartAll 启动所有渠道
func (m *ChannelManager) StartAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for id, ch := range m.channels {
		if err := ch.Start(ctx); err != nil {
			return fmt.Errorf("failed to start channel %s: %w", id, err)
		}
	}
	return nil
}

// StopAll 停止所有渠道
func (m *ChannelManager) StopAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, ch := range m.channels {
		ch.Stop()
	}
}

// ============================================================================
// Webhook 渠道
// ============================================================================

// WebhookConfig Webhook 渠道配置
type WebhookConfig struct {
	ID           string            // 渠道 ID
	URL          string            // 回调 URL
	Token        string            // 验证 Token
	Secret       string            // 签名密钥
	Headers      map[string]string // 自定义头
	MessageFormat string           // 消息格式: text, json
}

// WebhookChannel Webhook 渠道
type WebhookChannel struct {
	config *WebhookConfig
	client *http.Client
	server *http.Server
}

// NewWebhookChannel 创建 Webhook 渠道
func NewWebhookChannel(config *WebhookConfig) *WebhookChannel {
	return &WebhookChannel{
		config: config,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetType 获取渠道类型
func (c *WebhookChannel) GetType() ChannelType {
	return ChannelTypeWebhook
}

// GetID 获取渠道 ID
func (c *WebhookChannel) GetID() string {
	return c.config.ID
}

// Start 启动渠道服务
func (c *WebhookChannel) Start(ctx context.Context) error {
	// Webhook 渠道是被动接收的，不需要启动服务
	// Webhook URL 由外部系统调用
	fmt.Printf("📡 [Channel] Webhook channel %s configured: %s\n", c.config.ID, c.config.URL)
	return nil
}

// Stop 停止渠道服务
func (c *WebhookChannel) Stop() error {
	if c.server != nil {
		return c.server.Shutdown(context.Background())
	}
	return nil
}

// SendMessage 发送消息
func (c *WebhookChannel) SendMessage(to, content string) error {
	var body string

	switch c.config.MessageFormat {
	case "json":
		// JSON 格式
		msg := map[string]interface{}{
			"msgtype": "text",
			"text": map[string]string{
				"content": content,
			},
			"to": to,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}
		body = string(data)
	default:
		// 纯文本格式
		body = content
	}

	// 创建请求
	req, err := http.NewRequest("POST", c.config.URL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// 添加自定义头
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}

	// 签名
	if c.config.Secret != "" {
		signature := c.sign(body)
		req.Header.Set("X-Signature", signature)
	}

	// 发送请求
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to send message: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	fmt.Printf("📤 [Channel] Sent message via webhook %s\n", c.config.ID)
	return nil
}

// HandleWebhook 处理 Webhook 回调
func (c *WebhookChannel) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// 验证 Token
	if c.config.Token != "" {
		token := r.Header.Get("X-Webhook-Token")
		if token != c.config.Token {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// 验证签名
	if c.config.Secret != "" {
		signature := r.Header.Get("X-Signature")
		expected := c.sign(string(body))
		if !hmac.Equal([]byte(signature), []byte(expected)) {
			http.Error(w, "Invalid Signature", http.StatusUnauthorized)
			return
		}
	}

	// 解析消息
	var msg map[string]interface{}
	if err := json.Unmarshal(body, &msg); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// 提取消息内容
	content := c.extractContent(msg)

	// 提取发送者
	from := c.extractSender(msg)

	fmt.Printf("📥 [Channel] Received webhook message from %s: %s\n", from, content)

	// 返回响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "ok",
		"message":  "received",
		"from":     from,
		"content":  content,
	})
}

// sign 生成签名
func (c *WebhookChannel) sign(body string) string {
	mac := hmac.New(sha256.New, []byte(c.config.Secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

// extractContent 从请求体中提取消息内容
func (c *WebhookChannel) extractContent(msg map[string]interface{}) string {
	// 尝试多种常见格式

	// 格式1: DingTalk/企业微信
	if text, ok := msg["text"].(map[string]interface{}); ok {
		if content, ok := text["content"].(string); ok {
			return content
		}
	}

	// 格式2: 通用
	if content, ok := msg["content"].(string); ok {
		return content
	}

	// 格式3: message
	if msgField, ok := msg["message"].(string); ok {
		return msgField
	}

	// 返回原始 JSON
	data, _ := json.Marshal(msg)
	return string(data)
}

// extractSender 从请求体中提取发送者
func (c *WebhookChannel) extractSender(msg map[string]interface{}) string {
	// 尝试多种常见格式

	// 格式1: user_id
	if userID, ok := msg["user_id"].(string); ok {
		return userID
	}

	// 格式2: from_user_name
	if from, ok := msg["from_user_name"].(string); ok {
		return from
	}

	// 格式3: sender
	if sender, ok := msg["sender"].(string); ok {
		return sender
	}

	return "unknown"
}

// ============================================================================
// Channel 配置
// ============================================================================

// ChannelConfig 渠道配置
type ChannelConfig struct {
	Type     ChannelType     `json:"type"`
	ID       string          `json:"id"`
	Enabled  bool           `json:"enabled"`
	Webhook  *WebhookConfig `json:"webhook,omitempty"`
	Telegram *TelegramConfig `json:"telegram,omitempty"`
	Slack    *SlackConfig   `json:"slack,omitempty"`
}

// TelegramConfig Telegram 配置
type TelegramConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

// SlackConfig Slack 配置
type SlackConfig struct {
	BotToken   string `json:"bot_token"`
	ChannelID string `json:"channel_id"`
	SigningSecret string `json:"signing_secret"`
}
