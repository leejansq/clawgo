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

package types

import "time"

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

// Announce 子 Agent 完成通知 (参考 OpenClaw)
type Announce struct {
	SessionKey string `json:"sessionKey"`
	Status     string `json:"status"` // completed/error
	Result     string `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
	Label      string `json:"label"`
	DurationMs int64  `json:"durationMs"`
}

// ============================================================================
// Gateway 类型定义
// ============================================================================

// GatewayConfig Gateway 配置
type GatewayConfig struct {
	Port     int    // 端口，默认 18789
	Bind     string // 绑定地址，127.0.0.1 或 0.0.0.0
	Token    string // 认证 Token
	Password string // 认证密码
}

// AuthMode 认证模式
type AuthMode string

const (
	AuthModeToken    AuthMode = "token"
	AuthModePassword AuthMode = "password"
	AuthModeNone     AuthMode = "none"
)

// ChatCompletionRequest OpenAI Chat Completion 请求
type ChatCompletionRequest struct {
	Model    string                   `json:"model"`
	Messages []map[string]interface{} `json:"messages"`
	Stream   bool                     `json:"stream,omitempty"`
}

// ChatCompletionResponse OpenAI Chat Completion 响应
type ChatCompletionResponse struct {
	ID      string      `json:"id"`
	Object  string      `json:"object"`
	Created int64       `json:"created"`
	Model   string      `json:"model"`
	Choices []Choice    `json:"choices"`
	Usage   Usage       `json:"usage"`
}

// Choice 选择
type Choice struct {
	Index        int         `json:"index"`
	Message      interface{} `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// Usage 使用量
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// SSEClient SSE 客户端
type SSEClient struct {
	ID         string
	SessionKey string
	Writer     interface{}
	Flusher    interface{}
	Done       chan struct{}
}

// ============================================================================
// Channel 类型定义
// ============================================================================

// ChannelType 渠道类型
type ChannelType string

const (
	ChannelTypeWebhook   ChannelType = "webhook"
	ChannelTypeTelegram ChannelType = "telegram"
	ChannelTypeSlack    ChannelType = "slack"
)

// ChannelConfig 渠道配置
type ChannelConfig struct {
	Type     ChannelType     `json:"type"`
	ID       string          `json:"id"`
	Enabled  bool            `json:"enabled"`
	Webhook  *WebhookConfig `json:"webhook,omitempty"`
	Telegram *TelegramConfig `json:"telegram,omitempty"`
	Slack    *SlackConfig   `json:"slack,omitempty"`
}

// WebhookConfig Webhook 渠道配置
type WebhookConfig struct {
	ID             string            // 渠道 ID
	URL            string            // 回调 URL
	Token          string            // 验证 Token
	Secret         string            // 签名密钥
	Headers        map[string]string // 自定义头
	MessageFormat  string            // 消息格式: text, json
}

// TelegramConfig Telegram 配置
type TelegramConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

// SlackConfig Slack 配置
type SlackConfig struct {
	BotToken      string `json:"bot_token"`
	ChannelID     string `json:"channel_id"`
	SigningSecret string `json:"signing_secret"`
}

// WebhookChannel Webhook 渠道
type WebhookChannel struct {
	Config *WebhookConfig
}

// ============================================================================
// Stock 类型定义
// ============================================================================

// QuoteResult 报价结果
type QuoteResult struct {
	Symbol         string  `json:"symbol"`
	RegularPrice   float64 `json:"regular_price"`
	RegularChange  float64 `json:"regular_change"`
	PercentChange  float64 `json:"percent_change"`
	Open           float64 `json:"open"`
	High           float64 `json:"high"`
	Low            float64 `json:"low"`
	Volume         int64   `json:"volume"`
	MarketCap      int64   `json:"market_cap"`
	PE             float64 `json:"pe"`
	EPS            float64 `json:"eps"`
	YearHigh       float64 `json:"year_high"`
	YearLow        float64 `json:"year_low"`
	Exchange       string  `json:"exchange"`
	Currency       string  `json:"currency"`
	Timestamp      int64   `json:"timestamp"`
}

// ChartResult K线数据
type ChartResult struct {
	Symbol    string     `json:"symbol"`
	Period    string     `json:"period"`
	Interval  string     `json:"interval"`
	Bars      []KLineBar `json:"bars"`
	StartTime int64      `json:"start_time"`
	EndTime   int64      `json:"end_time"`
}

// KLineBar K线柱
type KLineBar struct {
	Timestamp int64   `json:"timestamp"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    int64   `json:"volume"`
}

// SearchResult 搜索结果
type SearchResult struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Exchange string `json:"exchange"`
	Type     string `json:"type"`
}

// AnalysisResult 分析结果
type AnalysisResult struct {
	Symbol              string               `json:"symbol"`
	CurrentPrice        float64              `json:"current_price"`
	Change              float64              `json:"change"`
	ChangePercent       float64              `json:"change_percent"`
	Analysis            string               `json:"analysis"`
	Recommendation      Recommendation       `json:"recommendation"`
	TechnicalIndicators TechnicalIndicators  `json:"technical_indicators"`
	RiskLevel           string               `json:"risk_level"`
}

// Recommendation 投资建议
type Recommendation struct {
	Suitability string   `json:"suitability"`
	Rating      string   `json:"rating"`
	Reasons     []string `json:"reasons"`
	RiskLevel   string   `json:"risk_level"`
}

// TechnicalIndicators 技术指标
type TechnicalIndicators struct {
	MA5        float64 `json:"ma5"`
	MA10       float64 `json:"ma10"`
	MA20       float64 `json:"ma20"`
	MA60       float64 `json:"ma60"`
	RSI        float64 `json:"rsi"`
	MACD       string  `json:"macd"`
	Trend      string  `json:"trend"`
	Volatility string  `json:"volatility"`
}

// ============================================================================
// 工具请求/响应类型
// ============================================================================

// SpawnRequest Spawn 工具请求
type SpawnRequest struct {
	Label       string `json:"label"`
	Task        string `json:"task"`
	Instruction string `json:"instruction"`
	TimeoutSec  int    `json:"timeoutSec"`
	Cleanup     string `json:"cleanup"`
	Workspace   string `json:"workspace"`
}

// SpawnResponse Spawn 工具响应
type SpawnResponse struct {
	Status        string `json:"status"`
	SessionKey    string `json:"sessionKey"`
	Label         string `json:"label"`
	WorkspacePath string `json:"workspacePath"`
	Note          string `json:"note"`
	Error         string `json:"error,omitempty"`
}

// GetResultRequest 获取结果请求
type GetResultRequest struct {
	SessionKey string `json:"sessionKey"`
	Wait       bool   `json:"wait"`
	WaitSec    int    `json:"waitSec"`
}

// GetResultResponse 获取结果响应
type GetResultResponse struct {
	SessionKey    string `json:"sessionKey"`
	Status        string `json:"status"`
	Result        string `json:"result,omitempty"`
	Error         string `json:"error,omitempty"`
	DurationMs    int64  `json:"durationMs"`
	WorkspacePath string `json:"workspacePath,omitempty"`
	Note          string `json:"note,omitempty"`
}

// ListSessionsRequest 列出会话请求
type ListSessionsRequest struct {
	Status   string `json:"status"`
	Label    string `json:"label"`
	Limit    int    `json:"limit"`
	ActiveMs int    `json:"activeMs"`
}

// ListSessionsResponse 列出会话响应
type ListSessionsResponse struct {
	Sessions []*SubAgentInfo `json:"sessions"`
	Total    int            `json:"total"`
	Note     string         `json:"note"`
}
