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
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	// MaxResponseChars is the default max chars of thumbed-down response
	MaxResponseChars = 500
)

// BuildReflectionPrompt builds the reflection prompt from feedback
func BuildReflectionPrompt(params struct {
	ThumbedDownResponse *string
	UserComment         *string
	MaxResponseChars     int
}) string {
	if params.MaxResponseChars <= 0 {
		params.MaxResponseChars = MaxResponseChars
	}

	var parts []string
	parts = append(parts, "A user indicated your previous response wasn't helpful.")

	// Include the thumbed-down response
	if params.ThumbedDownResponse != nil && *params.ThumbedDownResponse != "" {
		truncated := *params.ThumbedDownResponse
		if len(truncated) > params.MaxResponseChars {
			truncated = truncated[:params.MaxResponseChars] + "..."
		}
		parts = append(parts, fmt.Sprintf("\nYour response was:\n> %s", truncated))
	}

	// Include user's comment if provided
	if params.UserComment != nil && *params.UserComment != "" {
		parts = append(parts, fmt.Sprintf("\nUser's comment: \"%s\"", *params.UserComment))
	}

	// Add instruction
	parts = append(parts, `
Briefly reflect: what could you improve? Consider tone, length,
accuracy, relevance, and specificity. Reply with a single JSON object
only, no markdown or prose, using this exact shape:
{"learning":"...","followUp":false,"userMessage":""}
- learning: a short internal adjustment note (1-2 sentences) for your future behavior in this conversation.
- followUp: true only if the user needs a direct follow-up message.
- userMessage: only the exact user-facing message to send; empty string when followUp is false.`)

	return strings.Join(parts, "")
}

// parseBooleanLike parses a value as boolean
func parseBooleanLike(v interface{}) (bool, bool) {
	switch val := v.(type) {
	case bool:
		return val, true
	case string:
		normalized := strings.TrimSpace(strings.ToLower(val))
		if normalized == "true" || normalized == "yes" {
			return true, true
		}
		if normalized == "false" || normalized == "no" {
			return false, true
		}
	}
	return false, false
}

// parseReflectionValue parses a raw JSON object into ParsedReflectionResponse
func parseReflectionValue(data interface{}) (*ParsedReflectionResponse, bool) {
	if data == nil {
		return nil, false
	}

	obj, ok := data.(map[string]interface{})
	if !ok {
		return nil, false
	}

	learning, ok := obj["learning"]
	if !ok {
		return nil, false
	}
	learningStr, ok := learning.(string)
	if !ok || strings.TrimSpace(learningStr) == "" {
		return nil, false
	}

	followUp, hasFollowUp := parseBooleanLike(obj["followUp"])
	if !hasFollowUp {
		followUp = false
	}

	var userMessage string
	if um, ok := obj["userMessage"].(string); ok && strings.TrimSpace(um) != "" {
		userMessage = strings.TrimSpace(um)
	}

	return &ParsedReflectionResponse{
		Learning:    strings.TrimSpace(learningStr),
		FollowUp:    followUp,
		UserMessage: userMessage,
	}, true
}

// ParseReflectionResponse parses the LLM's reflection response
func ParseReflectionResponse(text string) (*ParsedReflectionResponse, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, false
	}

	// Try candidates: raw text, and text extracted from markdown code blocks
	candidates := []string{trimmed}
	codeBlockRe := regexp.MustCompile("(?i)```(?:json)?\\s*([\\s\\S]*?)```")
	if codeBlockMatch := codeBlockRe.FindAllStringSubmatch(trimmed, -1); len(codeBlockMatch) > 0 {
		// Take the first code block content
		candidates = append(candidates, codeBlockMatch[0][1])
	}

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}

		// Try parsing as JSON
		var data interface{}
		if err := json.Unmarshal([]byte(candidate), &data); err == nil {
			if parsed, ok := parseReflectionValue(data); ok {
				return parsed, true
			}
		}
	}

	// Safe fallback: keep the raw text as learning, but never auto-message
	return &ParsedReflectionResponse{
		Learning:   trimmed,
		FollowUp:   false,
	}, true
}
