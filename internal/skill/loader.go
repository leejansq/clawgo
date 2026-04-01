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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// DefaultSkillDirs returns the default skill directories in priority order
func DefaultSkillDirs() []SkillSource {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()

	return []SkillSource{
		// Project-level skills (highest priority)
		{Path: filepath.Join(cwd, "workspace", "skills"), Priority: 100, Label: "workspace-skills"},
		{Path: filepath.Join(cwd, ".agents", "skills"), Priority: 100, Label: "agents-skills-project"},

		// Personal skills
		{Path: filepath.Join(home, ".agents", "skills"), Priority: 50, Label: "agents-skills-personal"},

		// Managed skills (installed packages)
		{Path: filepath.Join(home, ".openclaw", "skills"), Priority: 10, Label: "openclaw-managed"},
	}
}

// SkillLoader handles loading and caching skills
type SkillLoader struct {
	sources   []SkillSource
	cache     map[string]*SkillEntry
	cacheMu   sync.RWMutex
	cacheTime int64
	cacheTTL  int64 // nanoseconds
}

// NewSkillLoader creates a new skill loader with given sources
func NewSkillLoader(sources []SkillSource) *SkillLoader {
	return &SkillLoader{
		sources:  sources,
		cache:    make(map[string]*SkillEntry),
		cacheTTL: 5 * 60 * 1e9, // 5 minutes in nanoseconds
	}
}

// LoadAll loads all skills from all sources
func (l *SkillLoader) LoadAll() ([]*SkillEntry, error) {
	allSkills := make(map[string]*SkillEntry) // key: normalized name

	for _, source := range l.sources {
		skills, err := l.loadFromSource(source)
		if err != nil {
			// Log warning but continue
			continue
		}

		for _, skill := range skills {
			existing, exists := allSkills[skill.Skill.Name]
			if !exists {
				// New skill
				allSkills[skill.Skill.Name] = skill
			} else if skill.Skill.Metadata != nil && existing.Skill.Metadata != nil {
				if skill.Skill.Metadata.Priority > existing.Skill.Metadata.Priority {
					// Higher priority source replaces
					allSkills[skill.Skill.Name] = skill
				}
			}
		}
	}

	result := make([]*SkillEntry, 0, len(allSkills))
	for _, entry := range allSkills {
		result = append(result, entry)
	}

	// Sort by priority (highest first), then by name
	sort.Slice(result, func(i, j int) bool {
		pi := 0
		pj := 0
		if result[i].Skill.Metadata != nil {
			pi = result[i].Skill.Metadata.Priority
		}
		if result[j].Skill.Metadata != nil {
			pj = result[j].Skill.Metadata.Priority
		}
		if pi != pj {
			return pi > pj
		}
		return result[i].Skill.Name < result[j].Skill.Name
	})

	return result, nil
}

// loadFromSource loads skills from a single source directory
func (l *SkillLoader) loadFromSource(source SkillSource) ([]*SkillEntry, error) {
	dir := source.Path

	// Check if directory exists
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Skip non-existent directories
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var result []*SkillEntry
	for _, entry := range entries {
		if entry.IsDir() {
			// Check for SKILL.md in subdirectory
			skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
			if _, err := os.Stat(skillPath); err == nil {
				if skill, err := ParseSkillFile(skillPath); err == nil {
					skill.Source = source.Label
					result = append(result, &SkillEntry{
						Skill:    skill,
						Eligible: true,
						Reason:   "loaded",
					})
				}
			}
		} else if entry.Name() == "SKILL.md" {
			// SKILL.md directly in source directory (single-skill source)
			skillPath := filepath.Join(dir, entry.Name())
			if skill, err := ParseSkillFile(skillPath); err == nil {
				skill.Source = source.Label
				result = append(result, &SkillEntry{
					Skill:    skill,
					Eligible: true,
					Reason:   "loaded",
				})
			}
		}
	}

	return result, nil
}

// LoadFromFile loads a single skill from a specific file
func (l *SkillLoader) LoadFromFile(filePath string) (*SkillEntry, error) {
	skill, err := ParseSkillFile(filePath)
	if err != nil {
		return nil, err
	}
	return &SkillEntry{
		Skill:    skill,
		Eligible: true,
		Reason:   "loaded",
	}, nil
}

// GetCached returns a cached skill by name
func (l *SkillLoader) GetCached(name string) *SkillEntry {
	l.cacheMu.RLock()
	defer l.cacheMu.RUnlock()
	return l.cache[name]
}

// SetCached caches a skill
func (l *SkillLoader) SetCached(name string, entry *SkillEntry) {
	l.cacheMu.Lock()
	defer l.cacheMu.Unlock()
	l.cache[name] = entry
}

// InvalidateCache clears the skill cache
func (l *SkillLoader) InvalidateCache() {
	l.cacheMu.Lock()
	defer l.cacheMu.Unlock()
	l.cache = make(map[string]*SkillEntry)
}

// AddSource adds a new skill source with given priority
func (l *SkillLoader) AddSource(path string, priority int, label string) {
	l.sources = append(l.sources, SkillSource{
		Path:     path,
		Priority: priority,
		Label:    label,
	})
	// Re-sort by priority
	sort.Slice(l.sources, func(i, j int) bool {
		return l.sources[i].Priority > l.sources[j].Priority
	})
	l.InvalidateCache() // Invalidate cache when sources change
}

// Global skill loader (thread-safe singleton)
var globalLoader *SkillLoader
var globalLoaderOnce sync.Once

// GetGlobalLoader returns the global skill loader instance
func GetGlobalLoader() *SkillLoader {
	globalLoaderOnce.Do(func() {
		globalLoader = NewSkillLoader(DefaultSkillDirs())
	})
	return globalLoader
}
