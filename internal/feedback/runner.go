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
 * WITHOUT WARRANTIES OF CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package feedback

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Runner handles the feedback reflection flow
type Runner struct {
	config *ReflectionConfig
}

// NewRunner creates a new feedback reflection runner
func NewRunner(cfg ...*ReflectionConfig) *Runner {
	config := DefaultReflectionConfig()
	if len(cfg) > 0 && cfg[0] != nil {
		config = cfg[0]
	}
	return &Runner{config: config}
}

// TriggerReflectionParams contains parameters for triggering a reflection
type TriggerReflectionParams struct {
	Feedback          *FeedbackEvent
	ThumbedDownResponse string   // The response that was thumbed down
	StorePath         string    // Path to store learnings files
	LLMReflect        func(ctx context.Context, prompt string) (string, error) // LLM call for reflection
	OnFollowUp        func(msg string) error // Optional callback for follow-up message
}

// TriggerReflection triggers a feedback reflection if conditions are met
func (r *Runner) TriggerReflection(ctx context.Context, params TriggerReflectionParams) error {
	// Only process negative feedback
	if params.Feedback == nil || params.Feedback.Value != FeedbackNegative {
		return nil
	}

	sessionKey := params.Feedback.SessionKey

	// Check cooldown
	if !IsReflectionAllowed(sessionKey, r.config.CooldownMs) {
		log.Printf("[feedback-reflection] Skipping reflection for session %s: cooldown active", sessionKey)
		return nil
	}

	// Record reflection time
	RecordReflectionTime(sessionKey, r.config.CooldownMs)

	// Build reflection prompt
	var userComment *string
	if params.Feedback.Comment != "" {
		userComment = &params.Feedback.Comment
	}
	var responsePtr *string
	if params.ThumbedDownResponse != "" {
		responsePtr = &params.ThumbedDownResponse
	}

	prompt := BuildReflectionPrompt(struct {
		ThumbedDownResponse *string
		UserComment         *string
		MaxResponseChars     int
	}{
		ThumbedDownResponse: responsePtr,
		UserComment:         userComment,
		MaxResponseChars:    r.config.MaxResponseChars,
	})

	// Run LLM reflection
	reflectionText, err := params.LLMReflect(ctx, prompt)
	if err != nil {
		return fmt.Errorf("reflection LLM call failed: %w", err)
	}

	// Parse response
	parsed, ok := ParseReflectionResponse(reflectionText)
	if !ok {
		log.Printf("[feedback-reflection] Failed to parse reflection response for session %s", sessionKey)
		return fmt.Errorf("failed to parse reflection response")
	}

	// Store learning
	if err := StoreSessionLearning(struct {
		StorePath  string
		SessionKey string
		Learning   string
		MaxLearnings int
	}{
		StorePath:     params.StorePath,
		SessionKey:    sessionKey,
		Learning:      parsed.Learning,
		MaxLearnings:  r.config.MaxLearnings,
	}); err != nil {
		log.Printf("[feedback-reflection] Failed to store learning for session %s: %v", sessionKey, err)
		return fmt.Errorf("failed to store learning: %w", err)
	}

	log.Printf("[feedback-reflection] Stored learning for session %s: %s", sessionKey, parsed.Learning)

	// Handle follow-up if needed
	if parsed.FollowUp && parsed.UserMessage != "" && params.OnFollowUp != nil {
		if err := params.OnFollowUp(parsed.UserMessage); err != nil {
			log.Printf("[feedback-reflection] Failed to send follow-up for session %s: %v", sessionKey, err)
			return fmt.Errorf("failed to send follow-up: %w", err)
		}
		log.Printf("[feedback-reflection] Sent follow-up for session %s: %s", sessionKey, parsed.UserMessage)
	}

	return nil
}

// TriggerReflectionAsync triggers reflection in a fire-and-forget manner
func (r *Runner) TriggerReflectionAsync(params TriggerReflectionParams) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if err := r.TriggerReflection(ctx, params); err != nil {
			log.Printf("[feedback-reflection] Async reflection failed: %v", err)
		}
	}()
}

// InjectLearningsIntoSystemPrompt loads session learnings and formats them for injection
func (r *Runner) InjectLearningsIntoSystemPrompt(storePath, sessionKey string) string {
	learnings, err := LoadSessionLearnings(storePath, sessionKey)
	if err != nil {
		log.Printf("[feedback-reflection] Failed to load learnings: %v", err)
		return ""
	}
	return FormatLearningsForPrompt(learnings)
}
