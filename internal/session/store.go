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
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SessionStore manages the session transcript as a JSONL file
type SessionStore interface {
	// Session lifecycle
	// sessionFile is optional - if empty, will be generated from cwd
	CreateSession(cwd string) (sessionID string, err error)
	OpenSession(sessionFile string) error
	GetSessionFile() string
	GetSessionID() string

	// Entry operations
	AppendMessage(role, content string) (entryID string, err error)
	AppendCompaction(summary string, firstKeptEntryID string, tokensBefore int) (entryID string, err error)
	AppendModelChange(provider, modelID string) (entryID string, err error)
	AppendCustomEntry(customType string, data interface{}) (entryID string, err error)
	AppendLabelChange(targetID, label string) (entryID string, err error)

	// Query
	GetBranch() []SessionEntry        // Path from root to current leaf
	GetEntries() []SessionEntry       // All entries
	GetEntry(id string) SessionEntry  // Single entry by ID
	GetHeader() *SessionHeader

	// Tree operations
	Branch(branchFromID string) error // Start new branch from entry
	ResetLeaf() error                 // Reset to root (current leaf)

	// Stats
	GetStats() *SessionStats
	Persist() error
	Close() error
}

// SessionEntry is a tagged union for session entries
type SessionEntry interface {
	GetID() string
	GetParentID() *string
	GetType() EntryType
	GetTimestamp() time.Time
}

// GetID returns the entry ID
func (e *MessageEntry) GetID() string { return e.ID }

// GetParentID returns the parent ID
func (e *MessageEntry) GetParentID() *string { return e.ParentID }

// GetType returns the entry type
func (e *MessageEntry) GetType() EntryType { return e.Type }

// GetTimestamp returns the timestamp
func (e *MessageEntry) GetTimestamp() time.Time { return e.Timestamp }

// GetID returns the entry ID
func (e *CompactionEntry) GetID() string { return e.ID }

// GetParentID returns the parent ID
func (e *CompactionEntry) GetParentID() *string { return e.ParentID }

// GetType returns the entry type
func (e *CompactionEntry) GetType() EntryType { return e.Type }

// GetTimestamp returns the timestamp
func (e *CompactionEntry) GetTimestamp() time.Time { return e.Timestamp }

// GetID returns the entry ID
func (e *ModelChangeEntry) GetID() string { return e.ID }

// GetParentID returns the parent ID
func (e *ModelChangeEntry) GetParentID() *string { return e.ParentID }

// GetType returns the entry type
func (e *ModelChangeEntry) GetType() EntryType { return e.Type }

// GetTimestamp returns the timestamp
func (e *ModelChangeEntry) GetTimestamp() time.Time { return e.Timestamp }

// GetID returns the entry ID
func (e *LabelEntry) GetID() string { return e.ID }

// GetParentID returns the parent ID
func (e *LabelEntry) GetParentID() *string { return e.ParentID }

// GetType returns the entry type
func (e *LabelEntry) GetType() EntryType { return e.Type }

// GetTimestamp returns the timestamp
func (e *LabelEntry) GetTimestamp() time.Time { return e.Timestamp }

// GetID returns the entry ID
func (e *CustomEntry) GetID() string { return e.ID }

// GetParentID returns the parent ID
func (e *CustomEntry) GetParentID() *string { return e.ParentID }

// GetType returns the entry type
func (e *CustomEntry) GetType() EntryType { return e.Type }

// GetTimestamp returns the timestamp
func (e *CustomEntry) GetTimestamp() time.Time { return e.Timestamp }

// GetID returns the entry ID
func (e *SessionInfoEntry) GetID() string { return e.ID }

// GetParentID returns the parent ID
func (e *SessionInfoEntry) GetParentID() *string { return e.ParentID }

// GetType returns the entry type
func (e *SessionInfoEntry) GetType() EntryType { return e.Type }

// GetTimestamp returns the timestamp
func (e *SessionInfoEntry) GetTimestamp() time.Time { return e.Timestamp }

// jsonSessionEntry is used for JSON unmarshaling with type discrimination
type jsonSessionEntry struct {
	ID        string          `json:"id"`
	ParentID  *string         `json:"parentId,omitempty"`
	Type      EntryType       `json:"type"`
	Timestamp string          `json:"timestamp"`
	Message   *MessageContent `json:"message,omitempty"`
	Summary   *string         `json:"summary,omitempty"`
	FirstKept *string         `json:"firstKeptEntryId,omitempty"`
	Tokens    *int            `json:"tokensBefore,omitempty"`
	Provider  *string         `json:"provider,omitempty"`
	ModelID   *string         `json:"modelId,omitempty"`
	TargetID  *string         `json:"targetId,omitempty"`
	Label     *string         `json:"label,omitempty"`
	CustomType *string        `json:"customType,omitempty"`
	Data      interface{}     `json:"data,omitempty"`
	Name      *string         `json:"name,omitempty"`
}

// memorySessionStore implements SessionStore with JSONL file storage
type memorySessionStore struct {
	mu          sync.RWMutex
	sessionFile string
	sessionID   string
	header      *SessionHeader
	entries     []SessionEntry
	currentLeaf string // ID of the current leaf entry
}

// NewSessionStore creates a new session store
func NewSessionStore() *memorySessionStore {
	return &memorySessionStore{
		entries: make([]SessionEntry, 0),
	}
}

// generateID generates a unique ID for entries
func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateSession creates a new session file
// Generates sessionFile from cwd as: {cwd}/.sessions/{sessionID}.jsonl
func (s *memorySessionStore) CreateSession(cwd string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessionID = generateID()

	// Generate session file path: {cwd}/.sessions/{sessionID}.jsonl
	s.sessionFile = filepath.Join(cwd, ".sessions", s.sessionID+".jsonl")

	s.header = &SessionHeader{
		Type:      "session",
		Version:   1,
		ID:        s.sessionID,
		Timestamp: time.Now().Format(time.RFC3339),
		CWD:       cwd,
	}

	// Create sessions directory
	dir := filepath.Dir(s.sessionFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create session directory: %w", err)
	}

	// Persist immediately
	if err := s.persistLocked(); err != nil {
		return "", err
	}

	return s.sessionID, nil
}

// OpenSession opens an existing session file
func (s *memorySessionStore) OpenSession(sessionFile string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessionFile = sessionFile

	file, err := os.Open(sessionFile)
	if err != nil {
		return fmt.Errorf("failed to open session file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		lineNum++

		if lineNum == 1 {
			// Parse header
			var header SessionHeader
			if err := json.Unmarshal([]byte(line), &header); err != nil {
				return fmt.Errorf("failed to parse session header: %w", err)
			}
			s.header = &header
			s.sessionID = header.ID
			continue
		}

		// Parse entry
		entry, err := parseSessionEntry([]byte(line))
		if err != nil {
			// Skip malformed entries
			continue
		}

		s.entries = append(s.entries, entry)

		// Track the current leaf (last entry in the main branch)
		if entry.GetParentID() == nil || *entry.GetParentID() == "" {
			s.currentLeaf = entry.GetID()
		}
	}

	// Find the actual leaf by traversing from root
	s.currentLeaf = s.findCurrentLeaf()

	return nil
}

// parseSessionEntry parses a JSON entry into the appropriate type
func parseSessionEntry(data []byte) (SessionEntry, error) {
	var base jsonSessionEntry
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, err
	}

	ts, _ := time.Parse(time.RFC3339, base.Timestamp)
	if ts.IsZero() {
		ts = time.Now()
	}

	switch base.Type {
	case EntryTypeMessage:
		if base.Message == nil {
			return nil, fmt.Errorf("message entry missing message field")
		}
		return &MessageEntry{
			BaseEntry: BaseEntry{
				ID:        base.ID,
				ParentID:  base.ParentID,
				Type:      base.Type,
				Timestamp: ts,
			},
			Message: *base.Message,
		}, nil

	case EntryTypeCompaction:
		summary := ""
		if base.Summary != nil {
			summary = *base.Summary
		}
		firstKept := ""
		if base.FirstKept != nil {
			firstKept = *base.FirstKept
		}
		tokens := 0
		if base.Tokens != nil {
			tokens = *base.Tokens
		}
		return &CompactionEntry{
			BaseEntry: BaseEntry{
				ID:        base.ID,
				ParentID:  base.ParentID,
				Type:      base.Type,
				Timestamp: ts,
			},
			Summary:          summary,
			FirstKeptEntryID: firstKept,
			TokensBefore:     tokens,
		}, nil

	case EntryTypeModelChange:
		provider := ""
		if base.Provider != nil {
			provider = *base.Provider
		}
		modelID := ""
		if base.ModelID != nil {
			modelID = *base.ModelID
		}
		return &ModelChangeEntry{
			BaseEntry: BaseEntry{
				ID:        base.ID,
				ParentID:  base.ParentID,
				Type:      base.Type,
				Timestamp: ts,
			},
			Provider: provider,
			ModelID:  modelID,
		}, nil

	case EntryTypeLabel:
		targetID := ""
		if base.TargetID != nil {
			targetID = *base.TargetID
		}
		label := ""
		if base.Label != nil {
			label = *base.Label
		}
		return &LabelEntry{
			BaseEntry: BaseEntry{
				ID:        base.ID,
				ParentID:  base.ParentID,
				Type:      base.Type,
				Timestamp: ts,
			},
			TargetID: targetID,
			Label:    label,
		}, nil

	case EntryTypeCustom:
		customType := ""
		if base.CustomType != nil {
			customType = *base.CustomType
		}
		return &CustomEntry{
			BaseEntry: BaseEntry{
				ID:        base.ID,
				ParentID:  base.ParentID,
				Type:      base.Type,
				Timestamp: ts,
			},
			CustomType: customType,
			Data:      base.Data,
		}, nil

	case EntryTypeSessionInfo:
		name := ""
		if base.Name != nil {
			name = *base.Name
		}
		return &SessionInfoEntry{
			BaseEntry: BaseEntry{
				ID:        base.ID,
				ParentID:  base.ParentID,
				Type:      base.Type,
				Timestamp: ts,
			},
			Name: name,
		}, nil

	case EntryTypeBranchSummary:
		summary := ""
		if base.Summary != nil {
			summary = *base.Summary
		}
		parentID := ""
		if base.ParentID != nil {
			parentID = *base.ParentID
		}
		return &BranchSummaryEntry{
			BaseEntry: BaseEntry{
				ID:        base.ID,
				ParentID:  &parentID,
				Type:      base.Type,
				Timestamp: ts,
			},
			ParentID: parentID,
			Summary:  summary,
		}, nil

	default:
		return nil, fmt.Errorf("unknown entry type: %s", base.Type)
	}
}

// findCurrentLeaf finds the current leaf by traversing from root
func (s *memorySessionStore) findCurrentLeaf() string {
	if len(s.entries) == 0 {
		return ""
	}

	// Build ID -> entry map
	entryByID := make(map[string]SessionEntry)
	var rootID string
	for _, e := range s.entries {
		entryByID[e.GetID()] = e
		if e.GetParentID() == nil || *e.GetParentID() == "" {
			rootID = e.GetID()
		}
	}

	// Traverse from root to leaf
	current := rootID
	for {
		children := s.getChildren(current, entryByID)
		if len(children) == 0 {
			return current
		}
		// Take the last child (most recent)
		current = children[len(children)-1].GetID()
	}
}

// getChildren returns children of a given entry
func (s *memorySessionStore) getChildren(parentID string, entryByID map[string]SessionEntry) []SessionEntry {
	var children []SessionEntry
	for _, e := range s.entries {
		if e.GetParentID() != nil && *e.GetParentID() == parentID {
			children = append(children, e)
		}
	}
	return children
}

// GetSessionFile returns the session file path
func (s *memorySessionStore) GetSessionFile() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionFile
}

// GetSessionID returns the session ID
func (s *memorySessionStore) GetSessionID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionID
}

// GetHeader returns the session header
func (s *memorySessionStore) GetHeader() *SessionHeader {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.header
}

// GetParentSession returns the parent session file path if this is a branched session
func (s *memorySessionStore) GetParentSession() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.header == nil {
		return ""
	}
	return s.header.ParentSession
}

// AppendMessage appends a message entry
func (s *memorySessionStore) AppendMessage(role, content string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	parentID := s.currentLeaf
	entry := NewMessageEntry(&parentID, role, content)
	s.entries = append(s.entries, entry)
	s.currentLeaf = entry.ID

	if err := s.persistLocked(); err != nil {
		return "", err
	}

	return entry.ID, nil
}

// AppendCompaction appends a compaction entry
func (s *memorySessionStore) AppendCompaction(summary string, firstKeptEntryID string, tokensBefore int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	parentID := s.currentLeaf
	entry := NewCompactionEntry(&parentID, summary, firstKeptEntryID, tokensBefore)
	s.entries = append(s.entries, entry)
	s.currentLeaf = entry.ID

	if err := s.persistLocked(); err != nil {
		return "", err
	}

	return entry.ID, nil
}

// AppendModelChange appends a model change entry
func (s *memorySessionStore) AppendModelChange(provider, modelID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	parentID := s.currentLeaf
	entry := NewModelChangeEntry(&parentID, provider, modelID)
	s.entries = append(s.entries, entry)
	s.currentLeaf = entry.ID

	if err := s.persistLocked(); err != nil {
		return "", err
	}

	return entry.ID, nil
}

// AppendCustomEntry appends a custom entry
func (s *memorySessionStore) AppendCustomEntry(customType string, data interface{}) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	parentID := s.currentLeaf
	entry := NewCustomEntry(&parentID, customType, data)
	s.entries = append(s.entries, entry)
	s.currentLeaf = entry.ID

	if err := s.persistLocked(); err != nil {
		return "", err
	}

	return entry.ID, nil
}

// AppendLabelChange appends a label entry
func (s *memorySessionStore) AppendLabelChange(targetID, label string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	parentID := s.currentLeaf
	entry := NewLabelEntry(&parentID, targetID, label)
	s.entries = append(s.entries, entry)
	s.currentLeaf = entry.ID

	if err := s.persistLocked(); err != nil {
		return "", err
	}

	return entry.ID, nil
}

// GetBranch returns the path from root to current leaf
func (s *memorySessionStore) GetBranch() []SessionEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.entries) == 0 {
		return nil
	}

	// Build path from root to current leaf
	branch := make([]SessionEntry, 0)
	current := s.currentLeaf

	// Build reverse map
	idToEntry := make(map[string]SessionEntry)
	for _, e := range s.entries {
		idToEntry[e.GetID()] = e
	}

	// Traverse backwards
	for current != "" {
		if entry, ok := idToEntry[current]; ok {
			branch = append([]SessionEntry{entry}, branch...)
			if entry.GetParentID() == nil {
				break
			}
			current = *entry.GetParentID()
		} else {
			break
		}
	}

	return branch
}

// GetEntries returns all entries
func (s *memorySessionStore) GetEntries() []SessionEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]SessionEntry, len(s.entries))
	copy(result, s.entries)
	return result
}

// GetEntry returns a single entry by ID
func (s *memorySessionStore) GetEntry(id string) SessionEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, e := range s.entries {
		if e.GetID() == id {
			return e
		}
	}
	return nil
}

// Branch creates a new branch from the specified entry
// Per OpenClaw's pattern, this creates a NEW session file (not entries in same file)
func (s *memorySessionStore) Branch(branchFromID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify the entry exists
	found := false
	for _, e := range s.entries {
		if e.GetID() == branchFromID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("entry not found: %s", branchFromID)
	}

	// Create a new branched session
	newStore, err := s.createBranchedSessionLocked(branchFromID)
	if err != nil {
		return err
	}

	// Copy over the new store's state
	s.sessionID = newStore.sessionID
	s.sessionFile = newStore.sessionFile
	s.header = newStore.header
	s.entries = newStore.entries
	s.currentLeaf = newStore.currentLeaf

	return nil
}

// createBranchedSessionLocked creates a new branched session (must be called with lock held)
// The new session file references the parent via ParentSession header field
func (s *memorySessionStore) createBranchedSessionLocked(branchFromID string) (*memorySessionStore, error) {
	// Generate new session ID
	newSessionID := generateID()

	// Generate new session file path in same directory
	newSessionFile := filepath.Join(filepath.Dir(s.sessionFile), newSessionID+".jsonl")

	// Create new header with ParentSession reference
	newHeader := &SessionHeader{
		Type:          "session",
		Version:       1,
		ID:            newSessionID,
		Timestamp:     time.Now().Format(time.RFC3339),
		CWD:           s.header.CWD,
		ParentSession: s.sessionFile, // Reference to parent session file
	}

	// Find the branch point entry to get its children
	branchPointChildren := s.getChildren(branchFromID, nil)

	// Build new entries starting from branch point children
	// Each child's parentID becomes nil (root of new branch)
	newEntries := make([]SessionEntry, 0)

	// Create a branch summary entry explaining this branch
	branchSummary := NewBranchSummaryEntry(nil, fmt.Sprintf("Branched from session %s at entry %s", s.sessionID, branchFromID))
	newEntries = append(newEntries, branchSummary)

	// Add children of branch point with parentID reset to branch summary
	for _, child := range branchPointChildren {
		switch e := child.(type) {
		case *MessageEntry:
			newEntry := &MessageEntry{
				BaseEntry: BaseEntry{
					ID:        generateID(),
					ParentID:  &branchSummary.ID,
					Type:      e.Type,
					Timestamp: e.Timestamp,
				},
				Message: e.Message,
			}
			newEntries = append(newEntries, newEntry)
		case *CompactionEntry:
			newEntry := &CompactionEntry{
				BaseEntry: BaseEntry{
					ID:        generateID(),
					ParentID:  &branchSummary.ID,
					Type:      e.Type,
					Timestamp: e.Timestamp,
				},
				Summary:          e.Summary,
				FirstKeptEntryID: e.FirstKeptEntryID,
				TokensBefore:     e.TokensBefore,
			}
			newEntries = append(newEntries, newEntry)
		case *ModelChangeEntry:
			newEntry := &ModelChangeEntry{
				BaseEntry: BaseEntry{
					ID:        generateID(),
					ParentID:  &branchSummary.ID,
					Type:      e.Type,
					Timestamp: e.Timestamp,
				},
				Provider: e.Provider,
				ModelID:  e.ModelID,
			}
			newEntries = append(newEntries, newEntry)
		case *LabelEntry:
			newEntry := &LabelEntry{
				BaseEntry: BaseEntry{
					ID:        generateID(),
					ParentID:  &branchSummary.ID,
					Type:      e.Type,
					Timestamp: e.Timestamp,
				},
				TargetID: e.TargetID,
				Label:    e.Label,
			}
			newEntries = append(newEntries, newEntry)
		case *CustomEntry:
			newEntry := &CustomEntry{
				BaseEntry: BaseEntry{
					ID:        generateID(),
					ParentID:  &branchSummary.ID,
					Type:      e.Type,
					Timestamp: e.Timestamp,
				},
				CustomType: e.CustomType,
				Data:       e.Data,
			}
			newEntries = append(newEntries, newEntry)
		case *SessionInfoEntry:
			newEntry := &SessionInfoEntry{
				BaseEntry: BaseEntry{
					ID:        generateID(),
					ParentID:  &branchSummary.ID,
					Type:      e.Type,
					Timestamp: e.Timestamp,
				},
				Name: e.Name,
			}
			newEntries = append(newEntries, newEntry)
		}
	}

	// Find current leaf (last entry with no children in new entries)
	newStore := &memorySessionStore{
		sessionFile: newSessionFile,
		sessionID:   newSessionID,
		header:      newHeader,
		entries:     newEntries,
	}
	newStore.currentLeaf = newStore.findCurrentLeaf()

	// Persist the new session file
	if err := newStore.persistLocked(); err != nil {
		return nil, err
	}

	return newStore, nil
}

// ResetLeaf resets to the root (null parent)
func (s *memorySessionStore) ResetLeaf() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find root (entry with no parent)
	for _, e := range s.entries {
		if e.GetParentID() == nil || *e.GetParentID() == "" {
			s.currentLeaf = e.GetID()
			return nil
		}
	}

	return fmt.Errorf("no root entry found")
}

// GetStats returns session statistics
func (s *memorySessionStore) GetStats() *SessionStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	compactionCount := 0
	for _, e := range s.entries {
		if e.GetType() == EntryTypeCompaction {
			compactionCount++
		}
	}

	return &SessionStats{
		TotalTokens:       estimateTokens(s.entries),
		TotalTokensFresh:  true,
		CompactionCount:   compactionCount,
		MemoryFlushCompactionCount: compactionCount, // Simplified: assume flushed
	}
}

// estimateTokens estimates total tokens in the session
// This follows OpenClaw's approach: count chars and divide by ~4
// Also adds JSON overhead for accurate estimation
func estimateTokens(entries []SessionEntry) int {
	totalChars := 0
	for _, e := range entries {
		switch entry := e.(type) {
		case *MessageEntry:
			// Count content plus estimated JSON overhead (~50 chars per entry)
			totalChars += len(entry.Message.Content) + 50
		case *CompactionEntry:
			// Count summary plus JSON overhead
			totalChars += len(entry.Summary) + 50
		case *BranchSummaryEntry:
			totalChars += len(entry.Summary) + 50
		case *LabelEntry:
			totalChars += len(entry.Label) + 30
		case *ModelChangeEntry:
			totalChars += len(entry.Provider) + len(entry.ModelID) + 30
		}
	}
	// Add base overhead for session structure
	totalChars += 200
	// Use 4 chars per token as baseline
	return totalChars / 4
}

// persistLocked writes the session to file (must be called with lock held)
func (s *memorySessionStore) persistLocked() error {
	if s.sessionFile == "" {
		return nil
	}

	// Write to temp file first, then rename (atomic write)
	tmpFile := s.sessionFile + ".tmp"

	file, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	// Write header
	headerBytes, _ := json.Marshal(s.header)
	if _, err := file.Write(headerBytes); err != nil {
		file.Close()
		os.Remove(tmpFile)
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := file.WriteString("\n"); err != nil {
		file.Close()
		os.Remove(tmpFile)
		return fmt.Errorf("failed to write newline: %w", err)
	}

	// Write entries
	for _, entry := range s.entries {
		var data []byte
		var err error

		switch e := entry.(type) {
		case *MessageEntry:
			data, err = json.Marshal(e)
		case *CompactionEntry:
			data, err = json.Marshal(e)
		case *ModelChangeEntry:
			data, err = json.Marshal(e)
		case *LabelEntry:
			data, err = json.Marshal(e)
		case *CustomEntry:
			data, err = json.Marshal(e)
		case *SessionInfoEntry:
			data, err = json.Marshal(e)
		case *BranchSummaryEntry:
			data, err = json.Marshal(e)
		}

		if err != nil {
			file.Close()
			os.Remove(tmpFile)
			return fmt.Errorf("failed to marshal entry: %w", err)
		}

		if _, err := file.Write(data); err != nil {
			file.Close()
			os.Remove(tmpFile)
			return fmt.Errorf("failed to write entry: %w", err)
		}
		if _, err := file.WriteString("\n"); err != nil {
			file.Close()
			os.Remove(tmpFile)
			return fmt.Errorf("failed to write newline: %w", err)
		}
	}

	file.Close()

	// Atomic rename
	if err := os.Rename(tmpFile, s.sessionFile); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// Persist persists the session to disk
func (s *memorySessionStore) Persist() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistLocked()
}

// Close closes the session store
func (s *memorySessionStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Persist any remaining changes
	if err := s.persistLocked(); err != nil {
		return err
	}

	s.entries = nil
	return nil
}

// TruncateEntries removes summarized entries after compaction
// This follows OpenClaw's session-truncation.ts logic:
// 1. Collect IDs of message entries that were summarized
// 2. Also remove labels whose targetId was removed
// 3. Also remove branch_summary entries whose parent was removed
// 4. Re-parent orphaned entries to nearest kept ancestor
func (s *memorySessionStore) TruncateEntries(compactionEntryID string, archivePath string) (*TruncationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.entries) == 0 {
		return &TruncationResult{
			Truncated: false,
			Reason:    "no entries",
		}, nil
	}

	// Find the compaction entry
	var compactionIdx = -1
	var compaction *CompactionEntry
	for i := len(s.entries) - 1; i >= 0; i-- {
		if ce, ok := s.entries[i].(*CompactionEntry); ok {
			if ce.ID == compactionEntryID {
				compactionIdx = i
				compaction = ce
				break
			}
		}
	}

	if compactionIdx < 0 {
		return &TruncationResult{
			Truncated: false,
			Reason:    "compaction entry not found",
		}, nil
	}

	// Nothing to truncate if compaction is at root
	if compaction.ParentID == nil || *compaction.ParentID == "" {
		return &TruncationResult{
			Truncated: false,
			Reason:    "compaction already at root",
		}, nil
	}

	// Get file size before truncation
	fileInfo, err := os.Stat(s.sessionFile)
	bytesBefore := int64(0)
	if err == nil {
		bytesBefore = fileInfo.Size()
	}

	// Build ID -> entry map
	entryByID := make(map[string]SessionEntry)
	for _, e := range s.entries {
		entryByID[e.GetID()] = e
	}

	// Collect IDs of summarized message entries (everything before firstKeptEntryId)
	summarizedIDs := make(map[string]bool)
	foundFirstKept := false
	for i := 0; i < compactionIdx; i++ {
		if compaction.FirstKeptEntryID != "" && s.entries[i].GetID() == compaction.FirstKeptEntryID {
			foundFirstKept = true
			break
		}
		if !foundFirstKept && s.entries[i].GetType() == EntryTypeMessage {
			summarizedIDs[s.entries[i].GetID()] = true
		}
	}

	// Mark labels for removal if their targetId was removed
	for _, e := range s.entries {
		if le, ok := e.(*LabelEntry); ok {
			if summarizedIDs[le.TargetID] {
				summarizedIDs[le.ID] = true
			}
		}
	}

	// Mark branch_summary entries for removal if their parent was removed
	for _, e := range s.entries {
		if bse, ok := e.(*BranchSummaryEntry); ok {
			if summarizedIDs[bse.ParentID] {
				summarizedIDs[bse.ID] = true
			}
		}
	}

	// Build kept entries, re-parenting orphaned entries
	keptEntries := make([]SessionEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		if summarizedIDs[entry.GetID()] {
			continue // Skip removed entries
		}

		// Re-parent orphaned entries to nearest kept ancestor
		newParentID := entry.GetParentID()
		if newParentID != nil {
			for summarizedIDs[*newParentID] {
				// Find the parent and get its parent
				if parent, ok := entryByID[*newParentID]; ok {
					if parent.GetParentID() == nil {
						newParentID = nil
						break
					}
					newParentID = parent.GetParentID()
				} else {
					break
				}
			}
		}

		// Update parent ID if needed
		if newParentID != entry.GetParentID() {
			switch e := entry.(type) {
			case *MessageEntry:
				e.ParentID = newParentID
			case *CompactionEntry:
				e.ParentID = newParentID
			case *ModelChangeEntry:
				e.ParentID = newParentID
			case *LabelEntry:
				e.ParentID = newParentID
			case *CustomEntry:
				e.ParentID = newParentID
			case *SessionInfoEntry:
				e.ParentID = newParentID
			case *BranchSummaryEntry:
				if newParentID != nil {
					e.ParentID = *newParentID
				}
			}
		}

		keptEntries = append(keptEntries, entry)
	}

	entriesRemoved := len(s.entries) - len(keptEntries)

	// Archive original file if requested
	if archivePath != "" {
		archiveDir := filepath.Dir(archivePath)
		if err := os.MkdirAll(archiveDir, 0755); err == nil {
			src, err := os.Open(s.sessionFile)
			if err == nil {
				dst, err := os.Create(archivePath)
				if err == nil {
					buf := make([]byte, 32*1024)
					for {
						n, rErr := src.Read(buf)
						if n > 0 {
							dst.Write(buf[:n])
						}
						if rErr != nil {
							break
						}
					}
					dst.Close()
				}
				src.Close()
			}
		}
	}

	// Update internal entries
	s.entries = keptEntries

	// Update currentLeaf to point to the compaction entry
	s.currentLeaf = compaction.ID

	// Persist the truncated session
	if err := s.persistLocked(); err != nil {
		return nil, err
	}

	// Estimate bytes after
	bytesAfter := int64(0)
	for _, e := range keptEntries {
		switch entry := e.(type) {
		case *MessageEntry:
			bytesAfter += int64(len(entry.Message.Content) + 100)
		case *CompactionEntry:
			bytesAfter += int64(len(entry.Summary) + 100)
		case *BranchSummaryEntry:
			bytesAfter += int64(len(entry.Summary) + 100)
		}
	}

	return &TruncationResult{
		Truncated:      entriesRemoved > 0,
		EntriesRemoved: entriesRemoved,
		BytesBefore:    bytesBefore,
		BytesAfter:     bytesAfter,
	}, nil
}
