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

	// Create main branch: A -> B -> C (where A is the root/first message)
	_, _ = store.AppendMessage("user", "A")
	idB, _ := store.AppendMessage("assistant", "B")
	_, _ = store.AppendMessage("user", "C")

	// Branch at B - this replaces the current store's state with the new branch
	// Following OpenClaw's createBranchedSession: path from root to B
	err = store.Branch(idB)
	if err != nil {
		t.Fatalf("Branch failed: %v", err)
	}

	// Should now have a new session file (branch)
	sessionFile := store.GetSessionFile()
	entries := store.GetEntries()

	// The branch session should have the path from root (A) to B: A -> B
	// Note: C is NOT included because B is the branch point
	if len(entries) != 2 {
		t.Errorf("branch should have 2 entries (A, B), got %d", len(entries))
	}

	// Verify the path structure: entries should be in order A -> B
	for i, entry := range entries {
		if i == 0 {
			// First entry should be root (A) with nil parent
			if entry.GetParentID() != nil && *entry.GetParentID() != "" {
				t.Errorf("entry[0] should be root with nil parent, got parent %s", *entry.GetParentID())
			}
			msg := entry.(*MessageEntry)
			if msg.Message.Content != "A" {
				t.Errorf("entry[0] content = %s, want A", msg.Message.Content)
			}
		} else {
			// Entry B should have parent pointing to A
			parentID := entry.GetParentID()
			if parentID == nil || *parentID == "" {
				t.Errorf("entry[%d] should have parent, got nil", i)
			} else if *parentID != entries[i-1].GetID() {
				t.Errorf("entry[%d] parent = %s, want %s", i, *parentID, entries[i-1].GetID())
			}
			msg := entry.(*MessageEntry)
			if msg.Message.Content != "B" {
				t.Errorf("entry[%d] content = %s, want B", i, msg.Message.Content)
			}
		}
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

func TestCreateBranch(t *testing.T) {
	tmpDir := tempTestDir(t)

	store := NewSessionStore()
	originalSessionID, err := store.CreateSession(tmpDir)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Create main branch: A -> B -> C (where A is the root/first message)
	_, _ = store.AppendMessage("user", "A")
	idB, _ := store.AppendMessage("assistant", "B")
	_, _ = store.AppendMessage("user", "C")

	// Keep track of original state
	originalSessionFile := store.GetSessionFile()
	originalEntryCount := len(store.GetEntries())

	// CreateBranch at B - should NOT modify original store
	// New session should contain the path from root (A) to B: A -> B
	// Note: Following OpenClaw's createBranchedSession semantics - C is NOT included
	branchStore, err := store.CreateBranch(idB)
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	if branchStore == nil {
		t.Fatal("CreateBranch returned nil store")
	}

	// Verify original store is UNCHANGED
	if store.GetSessionID() != originalSessionID {
		t.Errorf("original store sessionID changed to %s, want %s", store.GetSessionID(), originalSessionID)
	}
	if store.GetSessionFile() != originalSessionFile {
		t.Errorf("original store sessionFile changed to %s, want %s", store.GetSessionFile(), originalSessionFile)
	}
	if len(store.GetEntries()) != originalEntryCount {
		t.Errorf("original store entry count changed to %d, want %d", len(store.GetEntries()), originalEntryCount)
	}

	// Verify branch store is correct
	branchSessionID := branchStore.GetSessionID()
	if branchSessionID == originalSessionID {
		t.Error("branch store should have different sessionID than original")
	}
	branchSessionFile := branchStore.GetSessionFile()
	if branchSessionFile == originalSessionFile {
		t.Error("branch store should have different sessionFile than original")
	}

	// Branch should have path from root (A) to B: A -> B (2 entries)
	branchEntries := branchStore.GetEntries()
	if len(branchEntries) != 2 {
		t.Errorf("branch store should have 2 entries (A, B), got %d", len(branchEntries))
	}
	for _, entry := range branchStore.GetBranch() {
		t.Log(">>>", entry)
	}

	// First entry (A) should be root with nil parent
	if branchEntries[0].GetParentID() != nil && *branchEntries[0].GetParentID() != "" {
		t.Errorf("branchEntries[0] should be root with nil parent, got %s", *branchEntries[0].GetParentID())
	}
	msgA := branchEntries[0].(*MessageEntry)
	if msgA.Message.Content != "A" {
		t.Errorf("branchEntries[0] content = %s, want A", msgA.Message.Content)
	}

	// Second entry (B) should have parent pointing to A
	if branchEntries[1].GetParentID() == nil || *branchEntries[1].GetParentID() != branchEntries[0].GetID() {
		t.Errorf("branchEntries[1] parent should be branchEntries[0], got %v", branchEntries[1].GetParentID())
	}
	msgB := branchEntries[1].(*MessageEntry)
	if msgB.Message.Content != "B" {
		t.Errorf("branchEntries[1] content = %s, want B", msgB.Message.Content)
	}

	// Check parent session reference in branch
	parentSession := branchStore.GetParentSession()
	if parentSession == "" {
		t.Error("branch store parentSession should not be empty")
	}
	if parentSession != originalSessionFile {
		t.Errorf("branch store parentSession = %s, want %s", parentSession, originalSessionFile)
	}
}

func TestCreateBranchFullPath(t *testing.T) {
	// Comprehensive test for createBranchedSessionLocked to verify it follows OpenClaw's
	// createBranchedSession(leafId) semantics:
	// "Create a new session file containing only the path from root to the specified leaf."
	tmpDir := tempTestDir(t)

	store := NewSessionStore()
	originalSessionID, err := store.CreateSession(tmpDir)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	originalSessionFile := store.GetSessionFile()

	// Create a complex session structure:
	// A (user) -> B (assistant) -> C (user) -> D (assistant) -> E (user)
	//                 |
	//                 +-> F (user) -> G (assistant) [branch from B]
	msgA, _ := store.AppendMessage("user", "Message A")
	msgB, _ := store.AppendMessage("assistant", "Message B")
	msgC, _ := store.AppendMessage("user", "Message C")
	msgD, _ := store.AppendMessage("assistant", "Message D")
	msgE, _ := store.AppendMessage("user", "Message E")

	// Test 1: Branch at root (A) - should only contain A
	t.Run("branch_at_root", func(t *testing.T) {
		branchStore, err := store.CreateBranch(msgA)
		if err != nil {
			t.Fatalf("CreateBranch at root failed: %v", err)
		}

		entries := branchStore.GetEntries()
		if len(entries) != 1 {
			t.Errorf("branch at root should have 1 entry, got %d", len(entries))
		}

		// Verify it's msgA
		msg := entries[0].(*MessageEntry)
		if msg.Message.Content != "Message A" {
			t.Errorf("entry content = %s, want Message A", msg.Message.Content)
		}

		// Verify parent is nil (root)
		if entries[0].GetParentID() != nil && *entries[0].GetParentID() != "" {
			t.Errorf("root entry should have nil parent, got %s", *entries[0].GetParentID())
		}
	})

	// Test 2: Branch at B - should contain A -> B (not C, D, E)
	t.Run("branch_at_B", func(t *testing.T) {
		branchStore, err := store.CreateBranch(msgB)
		if err != nil {
			t.Fatalf("CreateBranch at B failed: %v", err)
		}

		entries := branchStore.GetEntries()
		if len(entries) != 2 {
			t.Errorf("branch at B should have 2 entries (A, B), got %d", len(entries))
		}

		// A should be root
		if entries[0].GetParentID() != nil && *entries[0].GetParentID() != "" {
			t.Errorf("A should be root with nil parent, got %s", *entries[0].GetParentID())
		}
		msgA := entries[0].(*MessageEntry)
		if msgA.Message.Content != "Message A" {
			t.Errorf("entries[0] content = %s, want Message A", msgA.Message.Content)
		}

		// B should have parent A
		if entries[1].GetParentID() == nil || *entries[1].GetParentID() != entries[0].GetID() {
			t.Errorf("B's parent should be A")
		}
		msgB := entries[1].(*MessageEntry)
		if msgB.Message.Content != "Message B" {
			t.Errorf("entries[1] content = %s, want Message B", msgB.Message.Content)
		}

		// Verify branch session file is different
		if branchStore.GetSessionFile() == originalSessionFile {
			t.Error("branch session file should be different from original")
		}

		// Verify parent session reference
		if branchStore.GetParentSession() != originalSessionFile {
			t.Errorf("parentSession = %s, want %s", branchStore.GetParentSession(), originalSessionFile)
		}
	})

	// Test 3: Branch at D - should contain A -> B -> C -> D
	t.Run("branch_at_D", func(t *testing.T) {
		branchStore, err := store.CreateBranch(msgD)
		if err != nil {
			t.Fatalf("CreateBranch at D failed: %v", err)
		}

		entries := branchStore.GetEntries()
		if len(entries) != 4 {
			t.Errorf("branch at D should have 4 entries (A, B, C, D), got %d", len(entries))
		}

		// Verify path: A -> B -> C -> D
		expectedContents := []string{"Message A", "Message B", "Message C", "Message D"}
		for i, expected := range expectedContents {
			msg := entries[i].(*MessageEntry)
			if msg.Message.Content != expected {
				t.Errorf("entries[%d] content = %s, want %s", i, msg.Message.Content, expected)
			}
		}

		// Verify parent chain
		for i := 1; i < len(entries); i++ {
			parentID := entries[i].GetParentID()
			if parentID == nil || *parentID != entries[i-1].GetID() {
				t.Errorf("entries[%d] parent should be entries[%d]", i, i-1)
			}
		}
	})

	// Test 4: Branch at E (leaf) - should contain A -> B -> C -> D -> E
	t.Run("branch_at_E", func(t *testing.T) {
		branchStore, err := store.CreateBranch(msgE)
		if err != nil {
			t.Fatalf("CreateBranch at E failed: %v", err)
		}

		entries := branchStore.GetEntries()
		if len(entries) != 5 {
			t.Errorf("branch at E should have 5 entries (A, B, C, D, E), got %d", len(entries))
		}

		// Verify E is the leaf (currentLeaf)
		if branchStore.GetSessionFile() != "" {
			// The new store's currentLeaf should be E
			branch := branchStore.GetBranch()
			if len(branch) != 5 {
				t.Errorf("GetBranch should return 5 entries, got %d", len(branch))
			}
			lastEntry := branch[len(branch)-1].(*MessageEntry)
			if lastEntry.Message.Content != "Message E" {
				t.Errorf("leaf content = %s, want Message E", lastEntry.Message.Content)
			}
		}
	})

	// Test 5: Original session is not modified
	t.Run("original_unchanged", func(t *testing.T) {
		if store.GetSessionID() != originalSessionID {
			t.Errorf("original sessionID changed to %s", store.GetSessionID())
		}
		if store.GetSessionFile() != originalSessionFile {
			t.Errorf("original sessionFile changed to %s", store.GetSessionFile())
		}
		entries := store.GetEntries()
		if len(entries) != 5 {
			t.Errorf("original should still have 5 entries, got %d", len(entries))
		}
	})

	// Test 6: Entry IDs are regenerated (not same as original)
	t.Run("ids_regenerated", func(t *testing.T) {
		branchStore, err := store.CreateBranch(msgC)
		if err != nil {
			t.Fatalf("CreateBranch at C failed: %v", err)
		}

		branchEntries := branchStore.GetEntries()
		originalEntries := store.GetEntries()

		// All IDs should be different
		for _, branchEntry := range branchEntries {
			for _, origEntry := range originalEntries {
				if branchEntry.GetID() == origEntry.GetID() {
					t.Errorf("branch entry ID %s should not match original entry ID %s",
						branchEntry.GetID(), origEntry.GetID())
				}
			}
		}
	})

	// Test 7: Entry types preserved correctly
	t.Run("entry_types_preserved", func(t *testing.T) {
		// Add different entry types
		_, _ = store.AppendCompaction("compaction summary", msgC, 1000)
		modelID, _ := store.AppendModelChange("anthropic", "claude-3-sonnet")

		// Branch at model change entry
		branchStore, err := store.CreateBranch(modelID)
		if err != nil {
			t.Fatalf("CreateBranch at model change failed: %v", err)
		}

		entries := branchStore.GetEntries()
		// Should have: A, B, C, compaction, D, E, modelChange (7 entries)
		// Full path from root (A) to modelID
		if len(entries) != 7 {
			t.Errorf("branch should have 7 entries (full path A->B->C->compaction->D->E->modelChange), got %d", len(entries))
		}

		// Find and verify the model change entry
		foundModelChange := false
		for _, entry := range entries {
			if entry.GetType() == EntryTypeModelChange {
				foundModelChange = true
				mc := entry.(*ModelChangeEntry)
				if mc.Provider != "anthropic" || mc.ModelID != "claude-3-sonnet" {
					t.Errorf("model change entry has wrong provider/model: %s/%s",
						mc.Provider, mc.ModelID)
				}
			}
			if entry.GetType() == EntryTypeCompaction {
				ce := entry.(*CompactionEntry)
				if ce.Summary != "compaction summary" {
					t.Errorf("compaction entry has wrong summary: %s", ce.Summary)
				}
			}
		}
		if !foundModelChange {
			t.Error("model change entry not found in branch")
		}
	})
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
