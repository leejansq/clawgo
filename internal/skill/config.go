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
)

// Config holds skill configuration
type Config struct {
	Sources           []SkillSource
	MaxPromptChars   int
	CompactThreshold int
	FilterEnabled    bool
	CacheEnabled     bool
	CacheTTLSeconds  int
}

// DefaultConfig returns the default skill configuration
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()

	return &Config{
		Sources: []SkillSource{
			{Path: filepath.Join(cwd, "workspace", "skills"), Priority: 100, Label: "workspace-skills"},
			{Path: filepath.Join(cwd, ".agents", "skills"), Priority: 100, Label: "agents-skills-project"},
			{Path: filepath.Join(home, ".agents", "skills"), Priority: 50, Label: "agents-skills-personal"},
			{Path: filepath.Join(home, ".openclaw", "skills"), Priority: 10, Label: "openclaw-managed"},
		},
		MaxPromptChars:   MaxSkillsPromptChars,
		CompactThreshold: CompactThreshold,
		FilterEnabled:    true,
		CacheEnabled:     true,
		CacheTTLSeconds: 300, // 5 minutes
	}
}

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() *Config {
	cfg := DefaultConfig()

	if maxChars := os.Getenv("CLAWGO_SKILLS_MAX_CHARS"); maxChars != "" {
		var n int
		if _, err := fmt.Sscanf(maxChars, "%d", &n); err == nil && n > 0 {
			cfg.MaxPromptChars = n
		}
	}

	if filterEnabled := os.Getenv("CLAWGO_SKILLS_FILTER_ENABLED"); filterEnabled == "false" {
		cfg.FilterEnabled = false
	}

	if cacheEnabled := os.Getenv("CLAWGO_SKILLS_CACHE_ENABLED"); cacheEnabled == "false" {
		cfg.CacheEnabled = false
	}

	return cfg
}

// AddExtraSkillDirs adds extra skill directories from environment
func AddExtraSkillDirs(cfg *Config) *Config {
	extraDirs := os.Getenv("CLAWGO_EXTRA_SKILL_DIRS")
	if extraDirs == "" {
		return cfg
	}

	// Parse comma-separated list of paths
	paths := filepath.SplitList(extraDirs)
	for i, path := range paths {
		if path != "" {
			cfg.Sources = append(cfg.Sources, SkillSource{
				Path:     path,
				Priority: 200 + i, // Extra dirs have highest priority
				Label:    "extra-" + filepath.Base(path),
			})
		}
	}

	return cfg
}

// MergeConfigs merges multiple configs (later configs override earlier)
func MergeConfigs(base *Config, overrides ...*Config) *Config {
	result := *base

	for _, override := range overrides {
		if override == nil {
			continue
		}

		if len(override.Sources) > 0 {
			result.Sources = override.Sources
		}
		if override.MaxPromptChars > 0 {
			result.MaxPromptChars = override.MaxPromptChars
		}
		if override.CompactThreshold > 0 {
			result.CompactThreshold = override.CompactThreshold
		}
		result.FilterEnabled = override.FilterEnabled
		result.CacheEnabled = override.CacheEnabled
		if override.CacheTTLSeconds > 0 {
			result.CacheTTLSeconds = override.CacheTTLSeconds
		}
	}

	return &result
}
