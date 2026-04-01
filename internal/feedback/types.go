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

package feedback

import "time"

// FeedbackValue represents the feedback type
type FeedbackValue string

const (
	FeedbackPositive FeedbackValue = "positive"
	FeedbackNegative FeedbackValue = "negative"
)

// FeedbackEvent represents a user feedback event
type FeedbackEvent struct {
	Type      string        `json:"type"`       // "custom"
	Event     string        `json:"event"`      // "feedback"
	TS        time.Time     `json:"ts"`         // timestamp
	MessageID string        `json:"messageId"`  // message ID
	Value     FeedbackValue `json:"value"`      // "positive" or "negative"
	Comment   string        `json:"comment"`    // optional user comment
	SessionKey string       `json:"sessionKey"` // session identifier
	AgentID   string        `json:"agentId"`    // agent identifier
	ConversationID string   `json:"conversationId"` // conversation identifier
}

// ParsedReflectionResponse is the parsed result from LLM reflection
type ParsedReflectionResponse struct {
	Learning   string `json:"learning"`   // internal adjustment note
	FollowUp   bool   `json:"followUp"`  // whether to send follow-up to user
	UserMessage string `json:"userMessage,omitempty"` // user-facing message
}

// ReflectionConfig contains configuration for feedback reflection
type ReflectionConfig struct {
	// CooldownMs is the cooldown between reflections per session (default 5 minutes)
	CooldownMs int64
	// MaxLearnings is the maximum number of learnings to retain (default 10)
	MaxLearnings int
	// MaxResponseChars is the max chars of thumbed-down response to include (default 500)
	MaxResponseChars int
}

// DefaultReflectionConfig returns the default configuration
func DefaultReflectionConfig() *ReflectionConfig {
	return &ReflectionConfig{
		CooldownMs:       300_000, // 5 minutes
		MaxLearnings:     10,
		MaxResponseChars: 500,
	}
}
