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
	"context"
	"fmt"
	"sync"
)

// Integration provides helpers for integrating skills with the agent runner
type Integration struct {
	loader      *SkillLoader
	filters    []FilterFunc
	config     *Config
	promptCache string
	cacheMu    sync.RWMutex
}

// NewIntegration creates a new skill integration
func NewIntegration(cfg *Config) *Integration {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	loader := NewSkillLoader(cfg.Sources)
	filters := DefaultFilters()

	return &Integration{
		loader:   loader,
		filters:  filters,
		config:   cfg,
	}
}

// LoadAndFilter loads all skills and applies filters
func (i *Integration) LoadAndFilter(ctx context.Context) ([]*SkillEntry, error) {
	entries, err := i.loader.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to load skills: %w", err)
	}

	if i.config.FilterEnabled {
		filterCtx := DefaultFilterContext()
		// Could enhance filterCtx with values from context if needed
		entries = FilterSkills(entries, i.filters, filterCtx)
	}

	return entries, nil
}

// BuildWorkspaceSkillsPrompt builds the skills prompt for workspace
func (i *Integration) BuildWorkspaceSkillsPrompt(ctx context.Context) (string, error) {
	i.cacheMu.RLock()
	if i.config.CacheEnabled && i.promptCache != "" {
		defer i.cacheMu.RUnlock()
		return i.promptCache, nil
	}
	i.cacheMu.RUnlock()

	entries, err := i.LoadAndFilter(ctx)
	if err != nil {
		return "", err
	}

	prompt := BuildSkillsPrompt(entries, FormatFull)

	i.cacheMu.Lock()
	i.promptCache = prompt.Content
	i.cacheMu.Unlock()

	return prompt.Content, nil
}

// BuildWorkspaceSkillsPromptWithFilter builds prompt with custom skill filter
func (i *Integration) BuildWorkspaceSkillsPromptWithFilter(ctx context.Context, skillFilter []string) (string, error) {
	entries, err := i.LoadAndFilter(ctx)
	if err != nil {
		return "", err
	}

	// Apply skill filter if provided
	if len(skillFilter) > 0 {
		filtered := make([]*SkillEntry, 0)
		filterSet := make(map[string]bool)
		for _, name := range skillFilter {
			filterSet[name] = true
		}
		for _, entry := range entries {
			if filterSet[entry.Skill.Name] {
				filtered = append(filtered, entry)
			}
		}
		entries = filtered
	}

	prompt := BuildSkillsPrompt(entries, FormatFull)
	return prompt.Content, nil
}

// InvalidatePromptCache invalidates the cached skills prompt
func (i *Integration) InvalidatePromptCache() {
	i.cacheMu.Lock()
	i.promptCache = ""
	i.cacheMu.Unlock()
	i.loader.InvalidateCache()
}

// SetPromptCache sets a pre-built prompt cache
func (i *Integration) SetPromptCache(prompt string) {
	i.cacheMu.Lock()
	i.promptCache = prompt
	i.cacheMu.Unlock()
}

// GetCommandRegistry builds a command registry from eligible skills
func (i *Integration) GetCommandRegistry(ctx context.Context) (*CommandRegistry, error) {
	entries, err := i.LoadAndFilter(ctx)
	if err != nil {
		return nil, err
	}

	registry := NewCommandRegistry()
	specs := BuildCommandSpecsFromSkills(entries)

	for _, spec := range specs {
		if err := registry.Register(spec); err != nil {
			// Skip duplicates
			continue
		}
	}

	return registry, nil
}

// GetSkillContent returns the content of a specific skill by name
func (i *Integration) GetSkillContent(ctx context.Context, skillName string) (string, error) {
	entries, err := i.LoadAndFilter(ctx)
	if err != nil {
		return "", err
	}

	entry := GetSkillByName(entries, skillName)
	if entry == nil {
		return "", fmt.Errorf("skill not found: %s", skillName)
	}

	return FormatSkillForModel(entry), nil
}

// GetSkillFilePath returns the file path of a specific skill by name
func (i *Integration) GetSkillFilePath(skillName string) (string, error) {
	entries, err := i.loader.LoadAll()
	if err != nil {
		return "", err
	}

	entry := GetSkillByName(entries, skillName)
	if entry == nil {
		return "", fmt.Errorf("skill not found: %s", skillName)
	}

	return entry.Skill.FilePath, nil
}

// ListSkillNames returns all skill names (eligible only)
func (i *Integration) ListSkillNames(ctx context.Context) ([]string, error) {
	entries, err := i.LoadAndFilter(ctx)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Skill.Name)
	}

	return names, nil
}
