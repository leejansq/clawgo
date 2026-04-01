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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultCooldownMs is the default cooldown between reflections per session
	DefaultCooldownMs int64 = 300_000 // 5 minutes

	// MaxCooldownEntries is the maximum cooldown entries before pruning
	MaxCooldownEntries = 500

	// DefaultMaxLearnings is the default max learnings to retain
	DefaultMaxLearnings = 10
)

// lastReflectionBySession tracks last reflection time per session
var lastReflectionBySession = make(map[string]int64)
var cooldownMutex sync.RWMutex

// sanitizeSessionKey sanitizes a session key for use in filenames
func sanitizeSessionKey(sessionKey string) string {
	// Replace any characters that aren't alphanumeric, underscore, or hyphen with underscore
	var result strings.Builder
	for _, r := range sessionKey {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}
	return result.String()
}

// IsReflectionAllowed checks if a reflection is allowed (cooldown not active)
func IsReflectionAllowed(sessionKey string, cooldownMs ...int64) bool {
	cooldownMutex.RLock()
	defer cooldownMutex.RUnlock()

	cooldown := DefaultCooldownMs
	if len(cooldownMs) > 0 && cooldownMs[0] > 0 {
		cooldown = cooldownMs[0]
	}

	lastTime, exists := lastReflectionBySession[sessionKey]
	if !exists {
		return true
	}
	return time.Now().UnixMilli()-lastTime >= cooldown
}

// RecordReflectionTime records that a reflection was run for a session
func RecordReflectionTime(sessionKey string, cooldownMs ...int64) {
	cooldownMutex.Lock()
	defer cooldownMutex.Unlock()

	cooldown := DefaultCooldownMs
	if len(cooldownMs) > 0 && cooldownMs[0] > 0 {
		cooldown = cooldownMs[0]
	}

	lastReflectionBySession[sessionKey] = time.Now().UnixMilli()
	pruneExpiredCooldowns(cooldown)
}

// pruneExpiredCooldowns removes expired cooldown entries to prevent unbounded growth
func pruneExpiredCooldowns(cooldownMs int64) {
	if len(lastReflectionBySession) <= MaxCooldownEntries {
		return
	}

	now := time.Now().UnixMilli()
	for key, t := range lastReflectionBySession {
		if now-t >= cooldownMs {
			delete(lastReflectionBySession, key)
		}
	}
}

// ClearReflectionCooldowns clears all cooldown tracking (for tests)
func ClearReflectionCooldowns() {
	cooldownMutex.Lock()
	defer cooldownMutex.Unlock()
	lastReflectionBySession = make(map[string]int64)
}

// StoreSessionLearning stores a learning derived from feedback reflection
func StoreSessionLearning(params struct {
	StorePath  string
	SessionKey string
	Learning   string
	MaxLearnings int
}) error {
	if params.MaxLearnings <= 0 {
		params.MaxLearnings = DefaultMaxLearnings
	}

	safeKey := sanitizeSessionKey(params.SessionKey)
	learningsFile := filepath.Join(params.StorePath, safeKey+".learnings.json")

	// Ensure directory exists
	if err := os.MkdirAll(params.StorePath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Load existing learnings
	var learnings []string
	data, err := os.ReadFile(learningsFile)
	if err == nil {
		if err := json.Unmarshal(data, &learnings); err != nil || !isStringSlice(learnings) {
			learnings = []string{}
		}
	}

	// Append new learning
	learnings = append(learnings, params.Learning)

	// Keep only last N learnings
	if len(learnings) > params.MaxLearnings {
		learnings = learnings[len(learnings)-params.MaxLearnings:]
	}

	// Write back
	encoded, err := json.MarshalIndent(learnings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal learnings: %w", err)
	}

	if err := os.WriteFile(learningsFile, encoded, 0644); err != nil {
		return fmt.Errorf("failed to write learnings file: %w", err)
	}

	return nil
}

// isStringSlice checks if the interface{} is a []string
func isStringSlice(v interface{}) bool {
	_, ok := v.([]string)
	return ok
}

// LoadSessionLearnings loads session learnings for injection into system prompt
func LoadSessionLearnings(storePath, sessionKey string) ([]string, error) {
	safeKey := sanitizeSessionKey(sessionKey)
	learningsFile := filepath.Join(storePath, safeKey+".learnings.json")

	data, err := os.ReadFile(learningsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read learnings file: %w", err)
	}

	var learnings []string
	if err := json.Unmarshal(data, &learnings); err != nil {
		return nil, fmt.Errorf("failed to parse learnings file: %w", err)
	}

	return learnings, nil
}

// FormatLearningsForPrompt formats learnings for injection into system prompt
func FormatLearningsForPrompt(learnings []string) string {
	if len(learnings) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n## Session Learnings (from past feedback)\n")
	sb.WriteString("The following learnings from previous feedback should guide your responses:\n")

	for i, learning := range learnings {
		sb.WriteString(fmt.Sprintf("- %d. %s\n", i+1, learning))
	}

	return sb.String()
}
