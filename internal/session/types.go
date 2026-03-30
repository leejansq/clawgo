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
	"time"
)

// EntryType represents the type of a session entry
type EntryType string

const (
	EntryTypeMessage       EntryType = "message"
	EntryTypeCompaction    EntryType = "compaction"
	EntryTypeModelChange   EntryType = "model_change"
	EntryTypeLabel         EntryType = "label"
	EntryTypeCustom        EntryType = "custom"
	EntryTypeSessionInfo   EntryType = "session_info"
	EntryTypeBranchSummary EntryType = "branch_summary"
)

// SessionHeader is the first line of the JSONL session file
type SessionHeader struct {
	Type          string `json:"type"` // "session"
	Version       int    `json:"version"`
	ID            string `json:"id"`
	Timestamp     string `json:"timestamp"`
	CWD           string `json:"cwd"`
	ParentSession string `json:"parentSession,omitempty"` // For branched sessions, points to parent session file
}

// MessageContent represents the content of a message
type MessageContent struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// BaseEntry contains common fields for all session entries
type BaseEntry struct {
	ID        string     `json:"id"`
	ParentID  *string    `json:"parentId,omitempty"`
	Type      EntryType  `json:"type"`
	Timestamp time.Time  `json:"timestamp"`
}

// MessageEntry represents a user or assistant message
type MessageEntry struct {
	BaseEntry
	Message MessageContent `json:"message"`
}

// NewMessageEntry creates a new message entry
func NewMessageEntry(parentID *string, role, content string) *MessageEntry {
	return &MessageEntry{
		BaseEntry: BaseEntry{
			ID:        generateID(),
			ParentID:  parentID,
			Type:      EntryTypeMessage,
			Timestamp: time.Now(),
		},
		Message: MessageContent{
			Role:    role,
			Content: content,
		},
	}
}

// CompactionEntry records a compaction summary
type CompactionEntry struct {
	BaseEntry
	Summary          string `json:"summary"`
	FirstKeptEntryID string `json:"firstKeptEntryId"`
	TokensBefore     int    `json:"tokensBefore"`
}

// NewCompactionEntry creates a new compaction entry
func NewCompactionEntry(parentID *string, summary, firstKeptEntryID string, tokensBefore int) *CompactionEntry {
	return &CompactionEntry{
		BaseEntry: BaseEntry{
			ID:        generateID(),
			ParentID:  parentID,
			Type:      EntryTypeCompaction,
			Timestamp: time.Now(),
		},
		Summary:          summary,
		FirstKeptEntryID: firstKeptEntryID,
		TokensBefore:     tokensBefore,
	}
}

// ModelChangeEntry records a model change
type ModelChangeEntry struct {
	BaseEntry
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

// NewModelChangeEntry creates a new model change entry
func NewModelChangeEntry(parentID *string, provider, modelID string) *ModelChangeEntry {
	return &ModelChangeEntry{
		BaseEntry: BaseEntry{
			ID:        generateID(),
			ParentID:  parentID,
			Type:      EntryTypeModelChange,
			Timestamp: time.Now(),
		},
		Provider: provider,
		ModelID:  modelID,
	}
}

// LabelEntry represents a user-defined bookmark/label
type LabelEntry struct {
	BaseEntry
	TargetID string `json:"targetId"`
	Label    string `json:"label"`
}

// NewLabelEntry creates a new label entry
func NewLabelEntry(parentID *string, targetID, label string) *LabelEntry {
	return &LabelEntry{
		BaseEntry: BaseEntry{
			ID:        generateID(),
			ParentID:  parentID,
			Type:      EntryTypeLabel,
			Timestamp: time.Now(),
		},
		TargetID: targetID,
		Label:    label,
	}
}

// CustomEntry represents extension-specific data
type CustomEntry struct {
	BaseEntry
	CustomType string      `json:"customType"`
	Data       interface{} `json:"data"`
}

// NewCustomEntry creates a new custom entry
func NewCustomEntry(parentID *string, customType string, data interface{}) *CustomEntry {
	return &CustomEntry{
		BaseEntry: BaseEntry{
			ID:        generateID(),
			ParentID:  parentID,
			Type:      EntryTypeCustom,
			Timestamp: time.Now(),
		},
		CustomType: customType,
		Data:       data,
	}
}

// SessionInfoEntry represents session metadata
type SessionInfoEntry struct {
	BaseEntry
	Name string `json:"name"`
}

// NewSessionInfoEntry creates a new session info entry
func NewSessionInfoEntry(parentID *string, name string) *SessionInfoEntry {
	return &SessionInfoEntry{
		BaseEntry: BaseEntry{
			ID:        generateID(),
			ParentID:  parentID,
			Type:      EntryTypeSessionInfo,
			Timestamp: time.Now(),
		},
		Name: name,
	}
}

// BranchSummaryEntry records a branch summary (created during compaction)
type BranchSummaryEntry struct {
	BaseEntry
	ParentID string `json:"parentId"` // Note: BranchSummary uses string, not *string
	Summary  string `json:"summary"`
}

// NewBranchSummaryEntry creates a new branch summary entry
func NewBranchSummaryEntry(parentID *string, summary string) *BranchSummaryEntry {
	return &BranchSummaryEntry{
		BaseEntry: BaseEntry{
			ID:        generateID(),
			ParentID:  parentID,
			Type:      EntryTypeBranchSummary,
			Timestamp: time.Now(),
		},
		ParentID: stringPtrToString(parentID),
		Summary:  summary,
	}
}

// Helper to convert *string to string
func stringPtrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// StringPtr creates a *string from a string
func StringPtr(s string) *string {
	return &s
}

// GetID returns the entry ID
func (e *BranchSummaryEntry) GetID() string { return e.ID }

// GetParentID returns the parent ID
func (e *BranchSummaryEntry) GetParentID() *string { return StringPtr(e.ParentID) }

// GetType returns the entry type
func (e *BranchSummaryEntry) GetType() EntryType { return e.Type }

// GetTimestamp returns the timestamp
func (e *BranchSummaryEntry) GetTimestamp() time.Time { return e.Timestamp }

// SessionStats tracks session statistics for compaction decisions
type SessionStats struct {
	TotalTokens              int  `json:"totalTokens"`
	TotalTokensFresh        bool `json:"totalTokensFresh"`
	CompactionCount         int  `json:"compactionCount"`
	MemoryFlushCompactionCount int `json:"memoryFlushCompactionCount"`
}

// CompactionResult contains results from compaction
type CompactionResult struct {
	EntryID      string
	Summary      string
	FirstKeptID  string
	TokensBefore int
}

// TruncationResult contains results from session truncation
type TruncationResult struct {
	Truncated      bool  `json:"truncated"`
	EntriesRemoved int   `json:"entriesRemoved"`
	BytesBefore    int64 `json:"bytesBefore,omitempty"`
	BytesAfter     int64 `json:"bytesAfter,omitempty"`
	Reason        string `json:"reason,omitempty"`
}
