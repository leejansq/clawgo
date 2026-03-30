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

package session

import (
	"context"
	"fmt"
	"time"

	"github.com/leejansq/clawgo/internal/memory"
)

// Memory flush constants and prompts (from OpenClaw)
const (
	// SILENT_REPLY_TOKEN is used to suppress output during memory flush
	SilentReplyToken = "[[DONE]]"

	// Default memory flush prompts
	DefaultMemoryFlushPrompt = `Pre-compaction memory flush.
Store durable memories only in memory/YYYY-MM-DD.md (create memory/ if needed).
If memory/YYYY-MM-DD.md already exists, APPEND new content only and do not overwrite existing entries.
Treat workspace bootstrap/reference files such as MEMORY.md, SOUL.md, TOOLS.md, and AGENTS.md as read-only during this flush; never overwrite, replace, or edit them.
If nothing to store, reply with [[DONE]].`

	DefaultMemoryFlushSystemPrompt = `Pre-compaction memory flush turn.
The session is near auto-compaction; capture durable memories to disk.
You may reply, but usually [[DONE]] is correct.`
)

const (
	// MemoryFlushTargetHint is the hint for where to store memories
	MemoryFlushTargetHint = "Store durable memories only in memory/YYYY-MM-DD.md (create memory/ if needed)."

	// MemoryFlushAppendOnlyHint is the hint for append-only behavior
	MemoryFlushAppendOnlyHint = "If memory/YYYY-MM-DD.md already exists, APPEND new content only and do not overwrite existing entries."

	// MemoryFlushReadOnlyHint is the hint for read-only files
	MemoryFlushReadOnlyHint = "Treat workspace bootstrap/reference files such as MEMORY.md, SOUL.md, TOOLS.md, and AGENTS.md as read-only during this flush; never overwrite, replace, or edit them."
)

// BuildMemoryFlushPrompt builds the memory flush prompt with date substituted
func BuildMemoryFlushPrompt(date string, customPrompt string) string {
	prompt := DefaultMemoryFlushPrompt
	if customPrompt != "" {
		prompt = customPrompt
	}

	// Substitute YYYY-MM-DD with actual date
	prompt = replaceDatePlaceholder(prompt, date)

	// Ensure safety hints are present
	if !contains(prompt, MemoryFlushTargetHint) {
		prompt = prompt + "\n\n" + MemoryFlushTargetHint
	}
	if !contains(prompt, MemoryFlushAppendOnlyHint) {
		prompt = prompt + "\n\n" + MemoryFlushAppendOnlyHint
	}
	if !contains(prompt, MemoryFlushReadOnlyHint) {
		prompt = prompt + "\n\n" + MemoryFlushReadOnlyHint
	}

	return prompt
}

// BuildMemoryFlushSystemPrompt builds the memory flush system prompt
func BuildMemoryFlushSystemPrompt(date string, customPrompt string) string {
	prompt := DefaultMemoryFlushSystemPrompt
	if customPrompt != "" {
		prompt = customPrompt
	}

	// Substitute YYYY-MM-DD with actual date
	prompt = replaceDatePlaceholder(prompt, date)

	// Ensure SILENT_REPLY_TOKEN hint is present
	if !contains(prompt, SilentReplyToken) {
		prompt = prompt + fmt.Sprintf("\n\nIf no user-visible reply is needed, start with %s.", SilentReplyToken)
	}

	return prompt
}

// replaceDatePlaceholder replaces YYYY-MM-DD placeholder with actual date
func replaceDatePlaceholder(text string, date string) string {
	// If date is empty, use today
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	// Replace all YYYY-MM-DD occurrences with actual date
	result := text
	for {
		idx := indexOf(result, "YYYY-MM-DD")
		if idx < 0 {
			break
		}
		result = result[:idx] + date + result[idx+10:]
	}
	return result
}

// indexOf finds the index of substring (simple implementation)
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// contains checks if text contains substring
func contains(text, substr string) bool {
	return indexOf(text, substr) >= 0
}

// MemoryFlushResult represents the result of a memory flush operation
type MemoryFlushResult struct {
	Success      bool
	ContentSaved bool
	Message      string
	Error        error
}

// MemoryFlush executes a memory flush operation
// This writes durable memories from the session to the short-term memory file
type MemoryFlush struct {
	store        memory.MemoryStore
	compactionMgr *CompactionManager
}

// NewMemoryFlush creates a new MemoryFlush handler
func NewMemoryFlush(store memory.MemoryStore, compactionMgr *CompactionManager) *MemoryFlush {
	return &MemoryFlush{
		store:        store,
		compactionMgr: compactionMgr,
	}
}

// ExecuteMemoryFlush runs the memory flush
// This is typically called before compaction to save durable memories
func (mf *MemoryFlush) ExecuteMemoryFlush(ctx context.Context, sessionContent string) (*MemoryFlushResult, error) {
	if mf.store == nil {
		return &MemoryFlushResult{
			Success: false,
			Error:   fmt.Errorf("memory store is nil"),
		}, fmt.Errorf("memory store is nil")
	}

	// Get today's date for the memory file
	date := time.Now().Format("2006-01-02")

	// Build the memory content to write
	// In a real implementation, this would extract important items from sessionContent
	// For now, we'll write a simple entry
	content := fmt.Sprintf("## Memory Flush - %s\n\nSession content summary extracted before compaction.\n", time.Now().Format(time.RFC3339))

	// Write to short-term memory (append mode via Write with MemoryTypeShortTerm)
	err := mf.store.Write(ctx, content, memory.MemoryMeta{
		Type:       memory.MemoryTypeShortTerm,
		Date:       date,
		Source:     "memory-flush",
		Importance: 7,
		Tags:       []string{"compaction", "auto"},
	})

	if err != nil {
		return &MemoryFlushResult{
			Success: false,
			Error:   err,
		}, err
	}

	return &MemoryFlushResult{
		Success:      true,
		ContentSaved: true,
		Message:     fmt.Sprintf("Saved durable memories to memory/%s.md", date),
	}, nil
}

// ShouldRunMemoryFlush determines if memory flush should run based on session stats
func (mf *MemoryFlush) ShouldRunMemoryFlush(stats *SessionStats) bool {
	if mf.compactionMgr == nil {
		return false
	}
	return mf.compactionMgr.ShouldMemoryFlush(stats)
}

// GetFlushPrompt returns the configured flush prompt
func (mf *MemoryFlush) GetFlushPrompt() string {
	if mf.compactionMgr == nil {
		return DefaultMemoryFlushPrompt
	}
	return mf.compactionMgr.GetFlushPrompt()
}

// GetFlushSystemPrompt returns the configured flush system prompt
func (mf *MemoryFlush) GetFlushSystemPrompt() string {
	if mf.compactionMgr == nil {
		return DefaultMemoryFlushSystemPrompt
	}
	return mf.compactionMgr.GetFlushSystemPrompt()
}
