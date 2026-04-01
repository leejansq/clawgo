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

package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSkillContent(t *testing.T) {
	// Test with full frontmatter
	content := `---
name: github
description: Manage GitHub repositories
user-invocable: true
os: [darwin, linux]
bins: [gh]
env: [GITHUB_TOKEN]
---

# GitHub Skill

This skill helps with GitHub operations.`

	skill, err := ParseSkillContent(content, "/test/SKILL.md")
	if err != nil {
		t.Fatalf("ParseSkillContent failed: %v", err)
	}

	if skill.Name != "github" {
		t.Errorf("Expected name 'github', got %q", skill.Name)
	}
	if skill.Description != "Manage GitHub repositories" {
		t.Errorf("Expected description 'Manage GitHub repositories', got %q", skill.Description)
	}
	if !skill.UserInvocable {
		t.Error("Expected UserInvocable to be true")
	}
	if skill.Metadata == nil {
		t.Fatal("Expected Metadata to be non-nil")
	}
	if len(skill.Metadata.OS) != 2 {
		t.Errorf("Expected 2 OS entries, got %d", len(skill.Metadata.OS))
	}
	if len(skill.Metadata.Bins) != 1 || skill.Metadata.Bins[0] != "gh" {
		t.Errorf("Expected Bins [gh], got %v", skill.Metadata.Bins)
	}
	if len(skill.Metadata.Env) != 1 || skill.Metadata.Env[0] != "GITHUB_TOKEN" {
		t.Errorf("Expected Env [GITHUB_TOKEN], got %v", skill.Metadata.Env)
	}
	if skill.Content == "" {
		t.Error("Expected Content to be non-empty")
	}
}

func TestParseSkillContentNoFrontmatter(t *testing.T) {
	// Test without frontmatter - should use filename
	content := `# Just some skill content without frontmatter`

	skill, err := ParseSkillContent(content, "/test/my-cool-skill/SKILL.md")
	if err != nil {
		t.Fatalf("ParseSkillContent failed: %v", err)
	}

	if skill.Name != "my-cool-skill" {
		t.Errorf("Expected name 'my-cool-skill', got %q", skill.Name)
	}
	if skill.Description != "" {
		t.Errorf("Expected empty description, got %q", skill.Description)
	}
}

func TestParseSkillFile(t *testing.T) {
	// Create a temp SKILL.md file
	tmpDir := t.TempDir()
	skillPath := filepath.Join(tmpDir, "test-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0755); err != nil {
		t.Fatal(err)
	}

	content := `---
name: test-skill
description: A test skill
priority: 50
---

Test content here.`

	if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	skill, err := ParseSkillFile(skillPath)
	if err != nil {
		t.Fatalf("ParseSkillFile failed: %v", err)
	}

	if skill.Name != "test-skill" {
		t.Errorf("Expected name 'test-skill', got %q", skill.Name)
	}
	if skill.FilePath != skillPath {
		t.Errorf("Expected FilePath %q, got %q", skillPath, skill.FilePath)
	}
	if skill.Metadata == nil || skill.Metadata.Priority != 50 {
		t.Errorf("Expected Priority 50, got %v", skill.Metadata)
	}
}

func TestFilterOS(t *testing.T) {
	ctx := &FilterContext{OS: "darwin"}

	// Skill for darwin should pass
	entry := &SkillEntry{
		Skill: &Skill{
			Name:     "test",
			Metadata: &SkillMetadata{OS: []string{"darwin", "linux"}},
		},
		Eligible: true,
	}
	if !FilterOS(entry, ctx) {
		t.Error("Expected darwin skill to pass on darwin")
	}

	// Skill for windows should fail
	entry2 := &SkillEntry{
		Skill: &Skill{
			Name:     "test2",
			Metadata: &SkillMetadata{OS: []string{"windows"}},
		},
		Eligible: true,
	}
	if FilterOS(entry2, ctx) {
		t.Error("Expected windows skill to fail on darwin")
	}
	if entry2.Reason == "" {
		t.Error("Expected Reason to be set")
	}

	// No OS restriction should pass
	entry3 := &SkillEntry{
		Skill: &Skill{
			Name:     "test3",
			Metadata: &SkillMetadata{},
		},
		Eligible: true,
	}
	if !FilterOS(entry3, ctx) {
		t.Error("Expected skill without OS restriction to pass")
	}
}

func TestFilterEnv(t *testing.T) {
	ctx := &FilterContext{
		EnvVars: map[string]string{
			"GITHUB_TOKEN": "abc123",
		},
	}

	// Skill with required env should pass
	entry := &SkillEntry{
		Skill: &Skill{
			Name:     "test",
			Metadata: &SkillMetadata{Env: []string{"GITHUB_TOKEN"}},
		},
		Eligible: true,
	}
	if !FilterEnv(entry, ctx) {
		t.Error("Expected skill with GITHUB_TOKEN to pass")
	}

	// Skill with missing env should fail
	entry2 := &SkillEntry{
		Skill: &Skill{
			Name:     "test2",
			Metadata: &SkillMetadata{Env: []string{"MISSING_VAR"}},
		},
		Eligible: true,
	}
	if FilterEnv(entry2, ctx) {
		t.Error("Expected skill with missing env to fail")
	}
}

func TestBuildSkillsPrompt(t *testing.T) {
	entries := []*SkillEntry{
		{
			Skill: &Skill{
				Name:        "skill1",
				Description: "First skill",
				FilePath:     "/path/to/skill1/SKILL.md",
				Metadata:    &SkillMetadata{},
			},
			Eligible: true,
		},
		{
			Skill: &Skill{
				Name:        "skill2",
				Description: "Second skill",
				FilePath:     "/path/to/skill2/SKILL.md",
				Metadata:    &SkillMetadata{},
			},
			Eligible: true,
		},
	}

	prompt := BuildSkillsPrompt(entries, FormatFull)
	if prompt.Content == "" {
		t.Error("Expected non-empty prompt content")
	}
	if prompt.Truncated {
		t.Error("Expected not truncated")
	}
	if !contains(prompt.Content, "skill1") {
		t.Error("Expected prompt to contain skill1")
	}
	if !contains(prompt.Content, "skill2") {
		t.Error("Expected prompt to contain skill2")
	}
}

func TestBuildCompactSkillsPrompt(t *testing.T) {
	entries := []*SkillEntry{
		{
			Skill: &Skill{
				Name:        "compact-skill",
				Description: "A skill with lots of description text that should be hidden in compact mode",
				FilePath:     "/path/to/compact/SKILL.md",
				Metadata:    &SkillMetadata{},
			},
			Eligible: true,
		},
	}

	prompt := BuildSkillsPrompt(entries, FormatCompact)
	if !contains(prompt.Content, "compact-skill") {
		t.Error("Expected prompt to contain skill name")
	}
	if contains(prompt.Content, "description text") {
		t.Error("Compact format should not include description")
	}
}

func TestBuildSkillsPromptEmpty(t *testing.T) {
	prompt := BuildSkillsPrompt([]*SkillEntry{}, FormatFull)
	if prompt.Content != "" {
		t.Errorf("Expected empty content, got %q", prompt.Content)
	}
}

func TestGetEligibleSkills(t *testing.T) {
	entries := []*SkillEntry{
		{Skill: &Skill{Name: "a"}, Eligible: true},
		{Skill: &Skill{Name: "b"}, Eligible: false},
		{Skill: &Skill{Name: "c"}, Eligible: true},
	}

	eligible := GetEligibleSkills(entries)
	if len(eligible) != 2 {
		t.Errorf("Expected 2 eligible, got %d", len(eligible))
	}
}

func TestCommandRegistry(t *testing.T) {
	registry := NewCommandRegistry()

	spec := &SkillCommandSpec{
		Name:        "test-cmd",
		Description: "A test command",
		SkillName:   "test-skill",
	}

	err := registry.Register(spec)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Should be able to get it
	retrieved := registry.Get("test-cmd")
	if retrieved == nil {
		t.Fatal("Expected to retrieve registered command")
	}
	if retrieved.Name != "test-cmd" {
		t.Errorf("Expected name 'test-cmd', got %q", retrieved.Name)
	}

	// Normalized name should work too
	retrieved2 := registry.Get("/TEST-CMD")
	if retrieved2 == nil {
		t.Fatal("Expected to retrieve by normalized name")
	}

	// Non-existent should return nil
	retrieved3 := registry.Get("nonexistent")
	if retrieved3 != nil {
		t.Error("Expected nil for non-existent command")
	}
}

func TestSkillLoader(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test skill directories
	skill1Dir := filepath.Join(tmpDir, "skill1")
	skill2Dir := filepath.Join(tmpDir, "skill2")
	if err := os.MkdirAll(skill1Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(skill2Dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write SKILL.md files
	skill1Content := `---
name: skill-one
description: First skill
priority: 10
---

Content 1`
	skill2Content := `---
name: skill-two
description: Second skill
priority: 20
---

Content 2`

	if err := os.WriteFile(filepath.Join(skill1Dir, "SKILL.md"), []byte(skill1Content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill2Dir, "SKILL.md"), []byte(skill2Content), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewSkillLoader([]SkillSource{
		{Path: tmpDir, Priority: 100, Label: "test"},
	})

	entries, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("Expected 2 skills, got %d", len(entries))
	}

	// Check deduplication and priority - skill-two should come first due to higher priority
	if entries[0].Skill.Name != "skill-two" {
		t.Errorf("Expected first skill to be skill-two (priority 20), got %s", entries[0].Skill.Name)
	}
}

func TestGetSkillByName(t *testing.T) {
	entries := []*SkillEntry{
		{Skill: &Skill{Name: "find"}, Eligible: true},
		{Skill: &Skill{Name: "replace"}, Eligible: true},
	}

	found := GetSkillByName(entries, "replace")
	if found == nil {
		t.Fatal("Expected to find replace skill")
	}
	if found.Skill.Name != "replace" {
		t.Errorf("Expected skill name 'replace', got %q", found.Skill.Name)
	}

	notFound := GetSkillByName(entries, "nonexistent")
	if notFound != nil {
		t.Error("Expected nil for non-existent skill")
	}
}

func TestDefaultFilters(t *testing.T) {
	filters := DefaultFilters()
	if len(filters) == 0 {
		t.Error("Expected at least one filter")
	}
}

func TestFilterSkills(t *testing.T) {
	ctx := &FilterContext{
		OS: "linux",
		EnvVars: map[string]string{
			"API_KEY": "abc",
		},
	}

	entries := []*SkillEntry{
		{
			Skill: &Skill{
				Name:     "os-skill",
				Metadata: &SkillMetadata{OS: []string{"linux"}},
			},
			Eligible: true,
		},
		{
			Skill: &Skill{
				Name:     "env-skill",
				Metadata: &SkillMetadata{Env: []string{"API_KEY"}},
			},
			Eligible: true,
		},
		{
			Skill: &Skill{
				Name:     "both-pass",
				Metadata: &SkillMetadata{OS: []string{"linux"}, Env: []string{"API_KEY"}},
			},
			Eligible: true,
		},
		{
			Skill: &Skill{
				Name:     "os-fail",
				Metadata: &SkillMetadata{OS: []string{"windows"}},
			},
			Eligible: true,
		},
	}

	filtered := FilterSkills(entries, DefaultFilters(), ctx)

	// 3 should pass (os-skill, env-skill, both-pass)
	// 1 should fail (os-fail)
	passCount := 0
	for _, e := range filtered {
		if e.Eligible {
			passCount++
		}
	}
	if passCount != 3 {
		t.Errorf("Expected 3 eligible, got %d", passCount)
	}
}

func TestBuildCommandSpecsFromSkills(t *testing.T) {
	entries := []*SkillEntry{
		{
			Skill: &Skill{
				Name:        "user-cmd",
				Description: "A user command",
				UserInvocable: true,
			},
			Eligible: true,
		},
		{
			Skill: &Skill{
				Name:        "auto-only",
				Description: "Auto only",
				UserInvocable: false,
			},
			Eligible: true,
		},
	}

	specs := BuildCommandSpecsFromSkills(entries)
	if len(specs) != 1 {
		t.Errorf("Expected 1 spec (only user-invocable), got %d", len(specs))
	}
	if specs[0].Name != "user-cmd" {
		t.Errorf("Expected name 'user-cmd', got %q", specs[0].Name)
	}
}

func TestConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxPromptChars != MaxSkillsPromptChars {
		t.Errorf("Expected MaxPromptChars %d, got %d", MaxSkillsPromptChars, cfg.MaxPromptChars)
	}
	if !cfg.FilterEnabled {
		t.Error("Expected FilterEnabled to be true")
	}
}

func TestIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		Sources: []SkillSource{
			{Path: tmpDir, Priority: 100, Label: "test"},
		},
		FilterEnabled: false,
	}

	integration := NewIntegration(cfg)
	if integration == nil {
		t.Fatal("NewIntegration returned nil")
	}

	// Create a test skill
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: test-skill
description: A test skill
---

Test content`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	names, err := integration.ListSkillNames(nil)
	if err != nil {
		t.Fatalf("ListSkillNames failed: %v", err)
	}
	if len(names) != 1 {
		t.Errorf("Expected 1 skill name, got %d", len(names))
	}
}

func TestFormatSkillForModel(t *testing.T) {
	entry := &SkillEntry{
		Skill: &Skill{
			Name:        "test-skill",
			Description: "A test skill description",
			Content:     "## Instructions\nUse this skill carefully.",
			Metadata:    &SkillMetadata{},
		},
		Eligible: true,
	}

	formatted := FormatSkillForModel(entry)
	if !contains(formatted, "Skill: test-skill") {
		t.Error("Expected formatted skill to contain skill name")
	}
	if !contains(formatted, "test skill description") {
		t.Error("Expected formatted skill to contain description")
	}
	if !contains(formatted, "Instructions") {
		t.Error("Expected formatted skill to contain Instructions")
	}
}

// Helper
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
