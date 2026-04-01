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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// FilterFunc is a function that checks skill eligibility
type FilterFunc func(entry *SkillEntry, ctx *FilterContext) bool

// FilterContext provides context for filtering decisions
type FilterContext struct {
	OS       string // Current OS (darwin, linux, windows)
	Arch     string // Current architecture
	EnvVars  map[string]string
	Config   map[string]string
	BinPaths []string // Additional PATH locations
}

// DefaultFilterContext creates a filter context from current environment
func DefaultFilterContext() *FilterContext {
	ctx := &FilterContext{
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		EnvVars: make(map[string]string),
		Config:  make(map[string]string),
	}

	// Capture current environment
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			ctx.EnvVars[parts[0]] = parts[1]
		}
	}

	// Add common bin paths
	ctx.BinPaths = filepath.SplitList(os.Getenv("PATH"))

	return ctx
}

// DefaultFilters returns the default set of skill filters
func DefaultFilters() []FilterFunc {
	return []FilterFunc{
		FilterOS,
		FilterBins,
		FilterEnv,
		FilterConfig,
	}
}

// FilterOS filters skills based on operating system requirement
func FilterOS(entry *SkillEntry, ctx *FilterContext) bool {
	if entry.Skill.Metadata == nil || len(entry.Skill.Metadata.OS) == 0 {
		return true // No OS restriction
	}

	for _, os := range entry.Skill.Metadata.OS {
		if strings.EqualFold(strings.TrimSpace(os), ctx.OS) {
			return true
		}
	}

	entry.Reason = fmt.Sprintf("OS mismatch: skill requires %v, current is %s",
		entry.Skill.Metadata.OS, ctx.OS)
	entry.Eligible = false
	return false
}

// FilterBins filters skills based on required binary dependencies
func FilterBins(entry *SkillEntry, ctx *FilterContext) bool {
	if entry.Skill.Metadata == nil || len(entry.Skill.Metadata.Bins) == 0 {
		return true
	}

	for _, bin := range entry.Skill.Metadata.Bins {
		bin = strings.TrimSpace(bin)
		if bin == "" {
			continue
		}

		// Check if binary exists in PATH
		if !hasBinary(bin, ctx.BinPaths) {
			entry.Reason = fmt.Sprintf("missing required binary: %s", bin)
			entry.Eligible = false
			return false
		}
	}

	return true
}

// hasBinary checks if a binary exists in the given paths
func hasBinary(name string, paths []string) bool {
	// Check if name is an absolute path
	if filepath.IsAbs(name) {
		_, err := os.Stat(name)
		return err == nil
	}

	for _, path := range paths {
		binPath := filepath.Join(path, name)
		if _, err := os.Stat(binPath); err == nil {
			return true
		}
		// Try with common extensions on Windows
		if runtime.GOOS == "windows" {
			for _, ext := range []string{".exe", ".cmd", ".bat"} {
				if _, err := os.Stat(binPath + ext); err == nil {
					return true
				}
			}
		}
	}

	// Also try using "which" command
	cmd := exec.Command("which", name)
	if err := cmd.Run(); err == nil {
		return true
	}

	return false
}

// FilterEnv filters skills based on required environment variables
func FilterEnv(entry *SkillEntry, ctx *FilterContext) bool {
	if entry.Skill.Metadata == nil || len(entry.Skill.Metadata.Env) == 0 {
		return true
	}

	for _, env := range entry.Skill.Metadata.Env {
		env = strings.TrimSpace(env)
		if env == "" {
			continue
		}

		// Check if environment variable is set
		if _, exists := ctx.EnvVars[env]; !exists {
			entry.Reason = fmt.Sprintf("missing required env var: %s", env)
			entry.Eligible = false
			return false
		}
	}

	return true
}

// FilterConfig filters skills based on required configuration
func FilterConfig(entry *SkillEntry, ctx *FilterContext) bool {
	if entry.Skill.Metadata == nil || len(entry.Skill.Metadata.Config) == 0 {
		return true
	}

	for _, config := range entry.Skill.Metadata.Config {
		config = strings.TrimSpace(config)
		if config == "" {
			continue
		}

		// Check if config path exists or config key is set
		// Config can be a file path or a key name
		if strings.HasPrefix(config, "/") || strings.HasPrefix(config, ".") {
			// File path - check if exists
			if _, err := os.Stat(config); err != nil {
				entry.Reason = fmt.Sprintf("missing required config file: %s", config)
				entry.Eligible = false
				return false
			}
		} else {
			// Key name - check if in config map
			if _, exists := ctx.Config[config]; !exists {
				entry.Reason = fmt.Sprintf("missing required config: %s", config)
				entry.Eligible = false
				return false
			}
		}
	}

	return true
}

// FilterSkills applies all filters to a list of skills
func FilterSkills(entries []*SkillEntry, filters []FilterFunc, ctx *FilterContext) []*SkillEntry {
	result := make([]*SkillEntry, 0, len(entries))

	for _, entry := range entries {
		eligible := true
		for _, filter := range filters {
			if !filter(entry, ctx) {
				eligible = false
				break
			}
		}
		if eligible {
			entry.Eligible = true
			entry.Reason = "passed all filters"
		}
		result = append(result, entry)
	}

	return result
}

// GetEligibleSkills returns only the eligible skills from a list
func GetEligibleSkills(entries []*SkillEntry) []*SkillEntry {
	result := make([]*SkillEntry, 0)
	for _, entry := range entries {
		if entry.Eligible {
			result = append(result, entry)
		}
	}
	return result
}

// GetIneligibleSkills returns only the ineligible skills (for debugging)
func GetIneligibleSkills(entries []*SkillEntry) []*SkillEntry {
	result := make([]*SkillEntry, 0)
	for _, entry := range entries {
		if !entry.Eligible {
			result = append(result, entry)
		}
	}
	return result
}
