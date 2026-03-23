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
	"net/http"
	"sync"

	"github.com/leejansq/clawgo/pkg/types"
)

// Channel 消息渠道接口
type Channel interface {
	// GetType 获取渠道类型
	GetType() types.ChannelType
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

	for _, ch := range m.channels {
		if err := ch.Start(ctx); err != nil {
			return err
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
