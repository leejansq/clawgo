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
	"os"
	"path/filepath"
	"testing"
)

// Helper to create temp directory for tests
func tempTestDir(t *testing.T) string {
	tmpDir := t.TempDir()
	return tmpDir
}

func TestCreateSession(t *testing.T) {
	tmpDir := tempTestDir(t)

	store := NewSessionStore()
	sessionID, err := store.CreateSession(tmpDir)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if sessionID == "" {
		t.Fatal("sessionID should not be empty")
	}

	// Check session file was created
	sessionFile := store.GetSessionFile()
	if sessionFile == "" {
		t.Fatal("sessionFile should not be empty")
	}

	expectedPath := filepath.Join(tmpDir, ".sessions", sessionID+".jsonl")
	if sessionFile != expectedPath {
		t.Errorf("sessionFile = %s, want %s", sessionFile, expectedPath)
	}

	// Check file exists
	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		t.Error("session file should exist")
	}

	// Check header
	header := store.GetHeader()
	if header == nil {
		t.Fatal("header should not be nil")
	}
	if header.Type != "session" {
		t.Errorf("header.Type = %s, want session", header.Type)
	}
	if header.Version != 1 {
		t.Errorf("header.Version = %d, want 1", header.Version)
	}
	if header.ID != sessionID {
		t.Errorf("header.ID = %s, want %s", header.ID, sessionID)
	}
	if header.CWD != tmpDir {
		t.Errorf("header.CWD = %s, want %s", header.CWD, tmpDir)
	}
}

func TestOpenSession(t *testing.T) {
	tmpDir := tempTestDir(t)

	// Create a session first
	store1 := NewSessionStore()
	sessionID, err := store1.CreateSession(tmpDir)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Add some entries
	_, err = store1.AppendMessage("user", "Hello")
	if err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}
	_, err = store1.AppendMessage("assistant", "Hi there!")
	if err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}

	sessionFile := store1.GetSessionFile()

	// Open the session with a new store
	store2 := NewSessionStore()
	err = store2.OpenSession(sessionFile)
	if err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Verify session ID matches
	if store2.GetSessionID() != sessionID {
		t.Errorf("store2.GetSessionID() = %s, want %s", store2.GetSessionID(), sessionID)
	}

	// Verify entries were loaded
	entries := store2.GetEntries()
	if len(entries) != 2 {
		t.Errorf("len(entries) = %d, want 2", len(entries))
	}

	// Verify header
	header := store2.GetHeader()
	if header.ID != sessionID {
		t.Errorf("header.ID = %s, want %s", header.ID, sessionID)
	}
}

func TestAppendMessage(t *testing.T) {
	tmpDir := tempTestDir(t)

	store := NewSessionStore()
	_, err := store.CreateSession(tmpDir)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Append user message
	entryID1, err := store.AppendMessage("user", "Hello")
	if err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}
	if entryID1 == "" {
		t.Error("entryID1 should not be empty")
	}

	// Append assistant message
	entryID2, err := store.AppendMessage("assistant", "Hi!")
	if err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}
	if entryID2 == "" {
		t.Error("entryID2 should not be empty")
	}

	// Verify entries
	entries := store.GetEntries()
	if len(entries) != 2 {
		t.Errorf("len(entries) = %d, want 2", len(entries))
	}

	// Verify message content
	msg1 := entries[0].(*MessageEntry)
	if msg1.Message.Role != "user" {
		t.Errorf("msg1.Message.Role = %s, want user", msg1.Message.Role)
	}
	if msg1.Message.Content != "Hello" {
		t.Errorf("msg1.Message.Content = %s, want Hello", msg1.Message.Content)
	}

	msg2 := entries[1].(*MessageEntry)
	if msg2.Message.Role != "assistant" {
		t.Errorf("msg2.Message.Role = %s, want assistant", msg2.Message.Role)
	}
	if msg2.Message.Content != "Hi!" {
		t.Errorf("msg2.Message.Content = %s, want Hi!", msg2.Message.Content)
	}

	// Verify parent chain
	if msg2.ParentID == nil || *msg2.ParentID != entryID1 {
		t.Error("msg2.ParentID should point to msg1")
	}
}

func TestAppendCompaction(t *testing.T) {
	tmpDir := tempTestDir(t)

	store := NewSessionStore()
	_, err := store.CreateSession(tmpDir)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Append some messages first
	msgID, _ := store.AppendMessage("user", "Hello")
	compactionID, err := store.AppendCompaction("Summary of conversation", msgID, 1000)
	if err != nil {
		t.Fatalf("AppendCompaction failed: %v", err)
	}

	if compactionID == "" {
		t.Error("compactionID should not be empty")
	}

	// Verify compaction entry
	entries := store.GetEntries()
	if len(entries) != 2 {
		t.Errorf("len(entries) = %d, want 2", len(entries))
	}

	compaction := entries[1].(*CompactionEntry)
	if compaction.Summary != "Summary of conversation" {
		t.Errorf("compaction.Summary = %s, want 'Summary of conversation'", compaction.Summary)
	}
	if compaction.FirstKeptEntryID != msgID {
		t.Errorf("compaction.FirstKeptEntryID = %s, want %s", compaction.FirstKeptEntryID, msgID)
	}
	if compaction.TokensBefore != 1000 {
		t.Errorf("compaction.TokensBefore = %d, want 1000", compaction.TokensBefore)
	}
}

func TestGetBranch(t *testing.T) {
	tmpDir := tempTestDir(t)

	store := NewSessionStore()
	_, err := store.CreateSession(tmpDir)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Append messages: root -> A -> B -> C
	idA, _ := store.AppendMessage("user", "A")
	_, _ = store.AppendMessage("assistant", "B")
	_, _ = store.AppendMessage("user", "C")

	// GetBranch should return all entries from root to leaf
	branch := store.GetBranch()
	if len(branch) != 3 {
		t.Errorf("len(branch) = %d, want 3", len(branch))
	}

	// First entry should be root (A)
	if branch[0].GetID() != idA {
		t.Errorf("branch[0].GetID() = %s, want %s", branch[0].GetID(), idA)
	}

	// Last entry should be C
	if branch[2].GetID() != branch[len(branch)-1].GetID() {
		t.Error("last entry should be C")
	}
}

func TestBranch(t *testing.T) {
	tmpDir := tempTestDir(t)

	store := NewSessionStore()
	_, err := store.CreateSession(tmpDir)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Create main branch: root -> A -> B -> C
	_, _ = store.AppendMessage("user", "A")
	idB, _ := store.AppendMessage("assistant", "B")
	_, _ = store.AppendMessage("user", "C")

	// Branch at B
	err = store.Branch(idB)
	if err != nil {
		t.Fatalf("Branch failed: %v", err)
	}

	// Should now have a new session file (branch)
	sessionFile := store.GetSessionFile()
	entries := store.GetEntries()

	// The branch session should have BranchSummary + descendants of B
	// B's descendants are C
	if len(entries) < 1 {
		t.Fatalf("branch should have at least BranchSummary entry")
	}

	// First entry should be BranchSummary
	bs := entries[0].(*BranchSummaryEntry)
	if bs.Type != EntryTypeBranchSummary {
		t.Errorf("first entry type = %s, want %s", bs.Type, EntryTypeBranchSummary)
	}

	// Check parent session reference
	parentSession := store.GetParentSession()
	if parentSession == "" {
		t.Error("parentSession should not be empty for branched session")
	}

	// The original session file should still exist (unchanged)
	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		t.Error("branch session file should exist")
	}
}

func TestTruncateEntries(t *testing.T) {
	tmpDir := tempTestDir(t)

	store := NewSessionStore()
	_, err := store.CreateSession(tmpDir)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Append messages: M1 -> M2 -> M3
	_, _ = store.AppendMessage("user", "M1")
	idM2, _ := store.AppendMessage("assistant", "M2")
	_, _ = store.AppendMessage("user", "M3")

	// Append compaction at M3, keeping from M2
	compactionID, err := store.AppendCompaction("Summary", idM2, 500)
	if err != nil {
		t.Fatalf("AppendCompaction failed: %v", err)
	}

	// Truncate entries before M2 (M1 should be removed)
	result, err := store.TruncateEntries(compactionID, "")
	if err != nil {
		t.Fatalf("TruncateEntries failed: %v", err)
	}

	if !result.Truncated {
		t.Error("TruncateEntries should report truncated=true")
	}

	if result.EntriesRemoved != 1 {
		t.Errorf("EntriesRemoved = %d, want 1", result.EntriesRemoved)
	}

	// Verify remaining entries
	entries := store.GetEntries()
	if len(entries) != 3 {
		t.Errorf("len(entries) after truncate = %d, want 3 (M2, M3, compaction)", len(entries))
	}

	// M2 should now be at root (parentId = nil)
	m2 := entries[0].(*MessageEntry)
	if m2.ID != idM2 {
		t.Errorf("entries[0].ID = %s, want %s", m2.ID, idM2)
	}
	if m2.ParentID != nil && *m2.ParentID != "" {
		t.Errorf("M2 should be root, but ParentID = %v", m2.ParentID)
	}
}

func TestGetStats(t *testing.T) {
	tmpDir := tempTestDir(t)

	store := NewSessionStore()
	_, err := store.CreateSession(tmpDir)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Append some messages
	_, _ = store.AppendMessage("user", "Hello world")
	_, _ = store.AppendMessage("assistant", "Hi there!")
	_, _ = store.AppendCompaction("Summary", "", 100)

	stats := store.GetStats()
	if stats == nil {
		t.Fatal("GetStats returned nil")
	}

	if stats.TotalTokens == 0 {
		t.Error("TotalTokens should be > 0")
	}

	if stats.CompactionCount != 1 {
		t.Errorf("CompactionCount = %d, want 1", stats.CompactionCount)
	}
}

func TestPersist(t *testing.T) {
	tmpDir := tempTestDir(t)

	store := NewSessionStore()
	_, err := store.CreateSession(tmpDir)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Append message
	_, _ = store.AppendMessage("user", "Test")

	sessionFile := store.GetSessionFile()

	// Create new store and open the same file
	store2 := NewSessionStore()
	err = store2.OpenSession(sessionFile)
	if err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	entries := store2.GetEntries()
	if len(entries) != 1 {
		t.Errorf("len(entries) = %d, want 1", len(entries))
	}
}

func TestEntryTypes(t *testing.T) {
	tmpDir := tempTestDir(t)

	store := NewSessionStore()
	_, err := store.CreateSession(tmpDir)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Test ModelChange
	modelID, err := store.AppendModelChange("anthropic", "claude-3-sonnet")
	if err != nil {
		t.Fatalf("AppendModelChange failed: %v", err)
	}
	if modelID == "" {
		t.Error("modelID should not be empty")
	}

	// Test CustomEntry
	customID, err := store.AppendCustomEntry("test_type", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("AppendCustomEntry failed: %v", err)
	}
	if customID == "" {
		t.Error("customID should not be empty")
	}

	// Test LabelChange
	labelID, err := store.AppendLabelChange(modelID, "important")
	if err != nil {
		t.Fatalf("AppendLabelChange failed: %v", err)
	}
	if labelID == "" {
		t.Error("labelID should not be empty")
	}

	entries := store.GetEntries()
	if len(entries) != 3 {
		t.Errorf("len(entries) = %d, want 3", len(entries))
	}

	// Verify types
	if entries[0].GetType() != EntryTypeModelChange {
		t.Errorf("entries[0].GetType() = %s, want %s", entries[0].GetType(), EntryTypeModelChange)
	}
	if entries[1].GetType() != EntryTypeCustom {
		t.Errorf("entries[1].GetType() = %s, want %s", entries[1].GetType(), EntryTypeCustom)
	}
	if entries[2].GetType() != EntryTypeLabel {
		t.Errorf("entries[2].GetType() = %s, want %s", entries[2].GetType(), EntryTypeLabel)
	}
}

func TestResetLeaf(t *testing.T) {
	tmpDir := tempTestDir(t)

	store := NewSessionStore()
	_, err := store.CreateSession(tmpDir)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Create branch: root -> A -> B -> C
	idA, _ := store.AppendMessage("user", "A")
	_, _ = store.AppendMessage("assistant", "B")
	_, _ = store.AppendMessage("user", "C")

	// Reset to root
	err = store.ResetLeaf()
	if err != nil {
		t.Fatalf("ResetLeaf failed: %v", err)
	}

	// Branch should now only have A
	branch := store.GetBranch()
	if len(branch) != 1 {
		t.Errorf("len(branch) after ResetLeaf = %d, want 1", len(branch))
	}
	if branch[0].GetID() != idA {
		t.Errorf("branch[0].GetID() = %s, want %s", branch[0].GetID(), idA)
	}
}

func TestGetEntry(t *testing.T) {
	tmpDir := tempTestDir(t)

	store := NewSessionStore()
	_, err := store.CreateSession(tmpDir)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	id1, _ := store.AppendMessage("user", "Hello")
	_, _ = store.AppendMessage("assistant", "World")

	// GetEntry should return correct entry
	entry := store.GetEntry(id1)
	if entry == nil {
		t.Fatal("GetEntry returned nil")
	}

	msg := entry.(*MessageEntry)
	if msg.Message.Content != "Hello" {
		t.Errorf("msg.Message.Content = %s, want Hello", msg.Message.Content)
	}

	// Get non-existent entry should return nil
	entry = store.GetEntry("non_existent")
	if entry != nil {
		t.Error("GetEntry for non-existent should return nil")
	}
}
