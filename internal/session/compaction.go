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
)

// CompactionMode represents the compaction mode
type CompactionMode string

const (
	CompactionModeSafeguard CompactionMode = "safeguard"
)

// Default compaction settings
const (
	DefaultReserveTokensFloor     = 10000
	DefaultSoftThresholdTokens    = 4000
	DefaultForceFlushTranscriptBytes = 2 * 1024 * 1024 // 2MB
)

// CompactionConfig contains configuration for compaction
type CompactionConfig struct {
	Mode                       CompactionMode
	ReserveTokensFloor         int
	SoftThresholdTokens        int
	ForceFlushTranscriptBytes  int
	MemoryFlushEnabled         bool
	MemoryFlushPrompt          string
	MemoryFlushSystemPrompt    string
	PostCompactionSections     []string
	ContextWindowTokens        int
}

// DefaultCompactionConfig returns default configuration
func DefaultCompactionConfig() *CompactionConfig {
	return &CompactionConfig{
		Mode:                       CompactionModeSafeguard,
		ReserveTokensFloor:         DefaultReserveTokensFloor,
		SoftThresholdTokens:        DefaultSoftThresholdTokens,
		ForceFlushTranscriptBytes:  DefaultForceFlushTranscriptBytes,
		MemoryFlushEnabled:         true,
		MemoryFlushPrompt:           DefaultMemoryFlushPrompt,
		MemoryFlushSystemPrompt:     DefaultMemoryFlushSystemPrompt,
		PostCompactionSections:      []string{"Session Startup", "Red Lines"},
		ContextWindowTokens:        200000, // Default 200k context window
	}
}

// CompactionManager manages the compaction lifecycle
type CompactionManager struct {
	config *CompactionConfig
}

// NewCompactionManager creates a new CompactionManager
func NewCompactionManager(config *CompactionConfig) *CompactionManager {
	if config == nil {
		config = DefaultCompactionConfig()
	}
	return &CompactionManager{config: config}
}

// ShouldMemoryFlush determines if a memory flush should run before compaction
func (cm *CompactionManager) ShouldMemoryFlush(stats *SessionStats) bool {
	if stats == nil {
		return false
	}

	// Check if already flushed for current compaction cycle
	if stats.CompactionCount > 0 && stats.MemoryFlushCompactionCount >= stats.CompactionCount {
		return false
	}

	// Calculate threshold
	threshold := cm.config.ContextWindowTokens - cm.config.ReserveTokensFloor - cm.config.SoftThresholdTokens
	if threshold <= 0 {
		return false
	}

	// Check if tokens exceed threshold
	return stats.TotalTokens >= threshold
}

// ShouldCompact determines if compaction should run
func (cm *CompactionManager) ShouldCompact(stats *SessionStats) bool {
	if stats == nil {
		return false
	}

	if cm.config.Mode != CompactionModeSafeguard {
		return false
	}

	// Calculate threshold
	threshold := cm.config.ContextWindowTokens - cm.config.ReserveTokensFloor
	if threshold <= 0 {
		return false
	}

	// Check if tokens exceed threshold
	return stats.TotalTokens >= threshold
}

// TruncateSession truncates the session file after compaction
// This removes message entries that were summarized, keeping the file size bounded
// It delegates to the store's TruncateEntries method which handles:
// 1. Removing summarized message entries
// 2. Removing labels whose targetId was removed
// 3. Removing branch_summary entries whose parent was removed
// 4. Re-parenting orphaned entries to nearest kept ancestor
func (cm *CompactionManager) TruncateSession(ctx context.Context, store *memorySessionStore, compactionEntryID string, archivePath string) (*TruncationResult, error) {
	if store == nil {
		return &TruncationResult{
			Truncated: false,
			Reason:    "store is nil",
		}, nil
	}

	return store.TruncateEntries(compactionEntryID, archivePath)
}

// GetFlushPrompt returns the memory flush prompt
func (cm *CompactionManager) GetFlushPrompt() string {
	return cm.config.MemoryFlushPrompt
}

// GetFlushSystemPrompt returns the memory flush system prompt
func (cm *CompactionManager) GetFlushSystemPrompt() string {
	return cm.config.MemoryFlushSystemPrompt
}

// GetConfig returns the compaction config
func (cm *CompactionManager) GetConfig() *CompactionConfig {
	return cm.config
}

// resolveMemoryFlushRelativePath returns the path for memory flush
func ResolveMemoryFlushRelativePath() string {
	return fmt.Sprintf("memory/%s.md", GetTodayDateString())
}

// GetTodayDateString returns today's date in YYYY-MM-DD format
func GetTodayDateString() string {
	return time.Now().Format("2006-01-02")
}
