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

package channel

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
	"time"

	"github.com/leejansq/clawgo/pkg/types"
)

// WebhookChannel Webhook 渠道
type WebhookChannel struct {
	config *types.WebhookConfig
	client *http.Client
	server *http.Server
}

// NewWebhookChannel 创建 Webhook 渠道
func NewWebhookChannel(config *types.WebhookConfig) *WebhookChannel {
	return &WebhookChannel{
		config: config,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetType 获取渠道类型
func (c *WebhookChannel) GetType() types.ChannelType {
	return types.ChannelTypeWebhook
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
		"status":  "ok",
		"message": "received",
		"from":    from,
		"content": content,
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
