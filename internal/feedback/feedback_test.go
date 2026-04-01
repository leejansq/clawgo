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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsReflectionAllowed(t *testing.T) {
	ClearReflectionCooldowns()
	defer ClearReflectionCooldowns()

	sessionKey := "test-session-1"

	// First time should be allowed
	if !IsReflectionAllowed(sessionKey) {
		t.Error("Expected reflection to be allowed for new session")
	}

	// Record a reflection
	RecordReflectionTime(sessionKey)

	// Immediately after should NOT be allowed
	if IsReflectionAllowed(sessionKey) {
		t.Error("Expected reflection to NOT be allowed immediately after recording")
	}

	// With very short cooldown should not be allowed
	if IsReflectionAllowed(sessionKey, 10000) {
		t.Error("Expected reflection to NOT be allowed with 10s cooldown")
	}
}

func TestIsReflectionAllowedWithCustomCooldown(t *testing.T) {
	ClearReflectionCooldowns()
	defer ClearReflectionCooldowns()

	sessionKey := "test-session-2"

	// Record with 1 second cooldown
	RecordReflectionTime(sessionKey, 1000)

	// Immediately after should NOT be allowed with 1s cooldown
	if IsReflectionAllowed(sessionKey, 1000) {
		t.Error("Expected reflection to NOT be allowed immediately")
	}
}

func TestStoreAndLoadSessionLearnings(t *testing.T) {
	tmpDir := t.TempDir()
	sessionKey := "test-session-learnings"
	maxLearnings := 5

	// Store multiple learnings
	for i := 1; i <= 3; i++ {
		learning := "Learning item " + string(rune('0'+i))
		err := StoreSessionLearning(struct {
			StorePath  string
			SessionKey string
			Learning   string
			MaxLearnings int
		}{
			StorePath:     tmpDir,
			SessionKey:    sessionKey,
			Learning:      learning,
			MaxLearnings:  maxLearnings,
		})
		if err != nil {
			t.Fatalf("Failed to store learning: %v", err)
		}
	}

	// Load and verify
	learnings, err := LoadSessionLearnings(tmpDir, sessionKey)
	if err != nil {
		t.Fatalf("Failed to load learnings: %v", err)
	}

	if len(learnings) != 3 {
		t.Errorf("Expected 3 learnings, got %d", len(learnings))
	}

	// Verify content
	expected := []string{"Learning item 1", "Learning item 2", "Learning item 3"}
	for i, exp := range expected {
		if learnings[i] != exp {
			t.Errorf("Expected learnings[%d]=%q, got %q", i, exp, learnings[i])
		}
	}
}

func TestStoreSessionLearningMaxLimit(t *testing.T) {
	tmpDir := t.TempDir()
	sessionKey := "test-session-max"
	maxLearnings := 3

	// Store more than max
	for i := 1; i <= 5; i++ {
		err := StoreSessionLearning(struct {
			StorePath  string
			SessionKey string
			Learning   string
			MaxLearnings int
		}{
			StorePath:     tmpDir,
			SessionKey:    sessionKey,
			Learning:      "Learning " + string(rune('0'+i)),
			MaxLearnings:  maxLearnings,
		})
		if err != nil {
			t.Fatalf("Failed to store learning: %v", err)
		}
	}

	// Load and verify only last 3 are kept
	learnings, err := LoadSessionLearnings(tmpDir, sessionKey)
	if err != nil {
		t.Fatalf("Failed to load learnings: %v", err)
	}

	if len(learnings) != maxLearnings {
		t.Errorf("Expected %d learnings (max limit), got %d", maxLearnings, len(learnings))
	}

	// Should have learnings 3, 4, 5 (last 3)
	expected := []string{"Learning 3", "Learning 4", "Learning 5"}
	for i, exp := range expected {
		if learnings[i] != exp {
			t.Errorf("Expected learnings[%d]=%q, got %q", i, exp, learnings[i])
		}
	}
}

func TestLoadSessionLearningsNonexistent(t *testing.T) {
	tmpDir := t.TempDir()

	learnings, err := LoadSessionLearnings(tmpDir, "nonexistent-session")
	if err != nil {
		t.Fatalf("Expected no error for nonexistent session, got: %v", err)
	}

	if learnings != nil {
		t.Errorf("Expected nil learnings for nonexistent session, got %v", learnings)
	}
}

func TestSanitizeSessionKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with-dash", "with-dash"},
		{"with_underscore", "with_underscore"},
		{"with spaces", "with_spaces"},
		{"with/slash", "with_slash"},
		{"with:colon", "with_colon"},
		{"msteams:user1", "msteams_user1"},
		{"channel/123/msg", "channel_123_msg"},
	}

	for _, tc := range tests {
		result := sanitizeSessionKey(tc.input)
		if result != tc.expected {
			t.Errorf("sanitizeSessionKey(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestFormatLearningsForPrompt(t *testing.T) {
	tests := []struct {
		name      string
		learnings []string
		expected  string
	}{
		{
			name:      "empty",
			learnings: nil,
			expected:  "",
		},
		{
			name:      "empty slice",
			learnings: []string{},
			expected:  "",
		},
		{
			name:      "single learning",
			learnings: []string{"Be more concise"},
			expected:  "\n\n## Session Learnings (from past feedback)\nThe following learnings from previous feedback should guide your responses:\n- 1. Be more concise\n",
		},
		{
			name:      "multiple learnings",
			learnings: []string{"Be more concise", "Check facts before claiming"},
			expected:  "\n\n## Session Learnings (from past feedback)\nThe following learnings from previous feedback should guide your responses:\n- 1. Be more concise\n- 2. Check facts before claiming\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FormatLearningsForPrompt(tc.learnings)
			if result != tc.expected {
				t.Errorf("FormatLearningsForPrompt() = %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestBuildReflectionPrompt(t *testing.T) {
	// Test with minimal params
	prompt := BuildReflectionPrompt(struct {
		ThumbedDownResponse *string
		UserComment         *string
		MaxResponseChars     int
	}{
		ThumbedDownResponse: nil,
		UserComment:         nil,
		MaxResponseChars:    0,
	})

	if prompt == "" {
		t.Error("Expected non-empty prompt")
	}

	if !contains(prompt, "wasn't helpful") {
		t.Error("Expected prompt to mention 'wasn't helpful'")
	}
}

func TestBuildReflectionPromptWithResponse(t *testing.T) {
	response := "This is a very long response that should be truncated if it exceeds the max chars limit"
	prompt := BuildReflectionPrompt(struct {
		ThumbedDownResponse *string
		UserComment         *string
		MaxResponseChars     int
	}{
		ThumbedDownResponse: &response,
		UserComment:         nil,
		MaxResponseChars:    20,
	})

	// The response is 83 chars, max is 20. Truncation cuts at position 20,
	// which lands at a space, so we get "This is a very long " + "..."
	expectedTruncated := "This is a very long ..."
	if !contains(prompt, expectedTruncated) {
		t.Errorf("Expected prompt to contain %q", expectedTruncated)
	}
}

func TestBuildReflectionPromptWithComment(t *testing.T) {
	comment := "This was not helpful at all"
	prompt := BuildReflectionPrompt(struct {
		ThumbedDownResponse *string
		UserComment         *string
		MaxResponseChars     int
	}{
		ThumbedDownResponse: nil,
		UserComment:         &comment,
		MaxResponseChars:    500,
	})

	if !contains(prompt, "User's comment:") {
		t.Error("Expected prompt to contain 'User's comment:'")
	}
	if !contains(prompt, comment) {
		t.Error("Expected prompt to contain the comment text")
	}
}

func TestParseReflectionResponse(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectOK    bool
		expectLearn string
		expectFU    bool
	}{
		{
			name:        "valid simple",
			input:       `{"learning":"be more concise","followUp":false,"userMessage":""}`,
			expectOK:    true,
			expectLearn: "be more concise",
			expectFU:    false,
		},
		{
			name:        "valid with followUp",
			input:       `{"learning":"check facts","followUp":true,"userMessage":"I apologize for the error"}`,
			expectOK:    true,
			expectLearn: "check facts",
			expectFU:    true,
		},
		{
			name:        "valid in markdown code block",
			input:       "```json\n{\"learning\":\"test\",\"followUp\":false}\n```",
			expectOK:    true,
			expectLearn: "test",
		},
		{
			name:        "empty string",
			input:       "",
			expectOK:    false,
		},
		{
			name:        "whitespace only",
			input:       "   ",
			expectOK:    false,
		},
		{
			name:        "plain text fallback",
			input:       "This is a plain text reflection without JSON",
			expectOK:    true,
			expectLearn: "This is a plain text reflection without JSON",
			expectFU:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, ok := ParseReflectionResponse(tc.input)
			if ok != tc.expectOK {
				t.Errorf("ParseReflectionResponse() ok = %v, want %v", ok, tc.expectOK)
				return
			}
			if tc.expectOK && parsed != nil {
				if parsed.Learning != tc.expectLearn {
					t.Errorf("parsed.Learning = %q, want %q", parsed.Learning, tc.expectLearn)
				}
				if parsed.FollowUp != tc.expectFU {
					t.Errorf("parsed.FollowUp = %v, want %v", parsed.FollowUp, tc.expectFU)
				}
			}
		})
	}
}

func TestTriggerReflectionNegativeFeedback(t *testing.T) {
	ClearReflectionCooldowns()
	defer ClearReflectionCooldowns()

	tmpDir := t.TempDir()
	called := false

	runner := NewRunner()

	err := runner.TriggerReflection(context.Background(), TriggerReflectionParams{
		Feedback: &FeedbackEvent{
			Value:      FeedbackNegative,
			SessionKey: "test-trigger",
			Comment:    "test comment",
		},
		ThumbedDownResponse: "my response",
		StorePath:           tmpDir,
		LLMReflect: func(ctx context.Context, prompt string) (string, error) {
			called = true
			return `{"learning":"be more careful","followUp":false,"userMessage":""}`, nil
		},
	})

	if err != nil {
		t.Fatalf("TriggerReflection failed: %v", err)
	}

	if !called {
		t.Error("Expected LLMReflect to be called")
	}

	// Verify learning was stored
	learnings, err := LoadSessionLearnings(tmpDir, "test-trigger")
	if err != nil {
		t.Fatalf("Failed to load learnings: %v", err)
	}

	if len(learnings) != 1 {
		t.Errorf("Expected 1 learning, got %d", len(learnings))
	}
	if learnings[0] != "be more careful" {
		t.Errorf("Expected learning 'be more careful', got %q", learnings[0])
	}
}

func TestTriggerReflectionPositiveFeedback(t *testing.T) {
	runner := NewRunner()
	called := false

	err := runner.TriggerReflection(context.Background(), TriggerReflectionParams{
		Feedback: &FeedbackEvent{
			Value:      FeedbackPositive,
			SessionKey: "test-positive",
		},
		LLMReflect: func(ctx context.Context, prompt string) (string, error) {
			called = true
			return "", nil
		},
	})

	if err != nil {
		t.Fatalf("TriggerReflection failed: %v", err)
	}

	if called {
		t.Error("Expected LLMReflect to NOT be called for positive feedback")
	}
}

func TestTriggerReflectionCooldown(t *testing.T) {
	ClearReflectionCooldowns()
	defer ClearReflectionCooldowns()

	tmpDir := t.TempDir()
	callCount := 0

	runner := NewRunner(&ReflectionConfig{CooldownMs: 60000}) // 1 minute cooldown

	// First reflection should work
	err := runner.TriggerReflection(context.Background(), TriggerReflectionParams{
		Feedback: &FeedbackEvent{
			Value:      FeedbackNegative,
			SessionKey: "test-cooldown",
		},
		StorePath: tmpDir,
		LLMReflect: func(ctx context.Context, prompt string) (string, error) {
			callCount++
			return `{"learning":"test","followUp":false}`, nil
		},
	})

	if err != nil {
		t.Fatalf("First TriggerReflection failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected 1 LLM call, got %d", callCount)
	}

	// Second reflection should be blocked by cooldown
	err = runner.TriggerReflection(context.Background(), TriggerReflectionParams{
		Feedback: &FeedbackEvent{
			Value:      FeedbackNegative,
			SessionKey: "test-cooldown",
		},
		StorePath: tmpDir,
		LLMReflect: func(ctx context.Context, prompt string) (string, error) {
			callCount++
			return `{"learning":"test2","followUp":false}`, nil
		},
	})

	if err != nil {
		t.Fatalf("Second TriggerReflection failed: %v", err)
	}

	// Should still be 1 because of cooldown
	if callCount != 1 {
		t.Errorf("Expected 1 LLM call (cooldown should block), got %d", callCount)
	}
}

func TestTriggerReflectionWithFollowUp(t *testing.T) {
	ClearReflectionCooldowns()
	defer ClearReflectionCooldowns()

	tmpDir := t.TempDir()
	followUpCalled := false
	followUpMsg := ""

	runner := NewRunner()

	err := runner.TriggerReflection(context.Background(), TriggerReflectionParams{
		Feedback: &FeedbackEvent{
			Value:      FeedbackNegative,
			SessionKey: "test-followup",
		},
		StorePath:           tmpDir,
		LLMReflect: func(ctx context.Context, prompt string) (string, error) {
			return `{"learning":"check facts","followUp":true,"userMessage":"I apologize for the mistake"}`, nil
		},
		OnFollowUp: func(msg string) error {
			followUpCalled = true
			followUpMsg = msg
			return nil
		},
	})

	if err != nil {
		t.Fatalf("TriggerReflection failed: %v", err)
	}

	if !followUpCalled {
		t.Error("Expected OnFollowUp to be called")
	}
	if followUpMsg != "I apologize for the mistake" {
		t.Errorf("Expected followUpMsg 'I apologize for the mistake', got %q", followUpMsg)
	}
}

func TestInjectLearningsIntoSystemPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	sessionKey := "test-inject"

	// Store learnings first
	StoreSessionLearning(struct {
		StorePath  string
		SessionKey string
		Learning   string
		MaxLearnings int
	}{
		StorePath:     tmpDir,
		SessionKey:    sessionKey,
		Learning:      "Be more concise",
		MaxLearnings:  10,
	})
	StoreSessionLearning(struct {
		StorePath  string
		SessionKey string
		Learning   string
		MaxLearnings int
	}{
		StorePath:     tmpDir,
		SessionKey:    sessionKey,
		Learning:      "Verify facts before stating",
		MaxLearnings:  10,
	})

	runner := NewRunner()
	result := runner.InjectLearningsIntoSystemPrompt(tmpDir, sessionKey)

	if !contains(result, "Session Learnings") {
		t.Error("Expected result to contain 'Session Learnings'")
	}
	if !contains(result, "Be more concise") {
		t.Error("Expected result to contain 'Be more concise'")
	}
	if !contains(result, "Verify facts before stating") {
		t.Error("Expected result to contain 'Verify facts before stating'")
	}
}

func TestNewRunner(t *testing.T) {
	runner := NewRunner()
	if runner == nil {
		t.Fatal("NewRunner returned nil")
	}
	if runner.config == nil {
		t.Error("runner.config is nil")
	}
}

func TestDefaultReflectionConfig(t *testing.T) {
	cfg := DefaultReflectionConfig()
	if cfg.CooldownMs != 300_000 {
		t.Errorf("Expected CooldownMs 300000, got %d", cfg.CooldownMs)
	}
	if cfg.MaxLearnings != 10 {
		t.Errorf("Expected MaxLearnings 10, got %d", cfg.MaxLearnings)
	}
	if cfg.MaxResponseChars != 500 {
		t.Errorf("Expected MaxResponseChars 500, got %d", cfg.MaxResponseChars)
	}
}

func TestFeedbackEvent(t *testing.T) {
	event := FeedbackEvent{
		Type:           "custom",
		Event:          "feedback",
		TS:             time.Now(),
		MessageID:      "msg-123",
		Value:          FeedbackNegative,
		Comment:        "not helpful",
		SessionKey:     "session-abc",
		AgentID:        "agent-1",
		ConversationID: "conv-xyz",
	}

	if event.Value != FeedbackNegative {
		t.Errorf("Expected FeedbackNegative, got %v", event.Value)
	}
	if event.SessionKey != "session-abc" {
		t.Errorf("Expected SessionKey 'session-abc', got %v", event.SessionKey)
	}
}

func TestLearningsFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	sessionKey := "msteams:user1@channel"

	StoreSessionLearning(struct {
		StorePath  string
		SessionKey string
		Learning   string
		MaxLearnings int
	}{
		StorePath:     tmpDir,
		SessionKey:    sessionKey,
		Learning:      "test learning",
		MaxLearnings:  10,
	})

	// Verify file exists with sanitized name
	safeKey := sanitizeSessionKey(sessionKey)
	expectedFile := filepath.Join(tmpDir, safeKey+".learnings.json")

	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Errorf("Expected learnings file at %s", expectedFile)
	}
}

// Helper - using strings.Contains for correctness
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
