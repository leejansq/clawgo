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

import "time"

// SkillInvocationPolicy defines when a skill can be invoked
type SkillInvocationPolicy string

const (
	PolicyAlways    SkillInvocationPolicy = "always"
	PolicyModelOnly SkillInvocationPolicy = "model-only"
	PolicyNever     SkillInvocationPolicy = "never"
)

// SkillMetadata contains parsed metadata from SKILL.md frontmatter
type SkillMetadata struct {
	OS       []string               // Operating systems (e.g., ["darwin", "linux"])
	Bins     []string               // Required binary dependencies
	Env      []string               // Required environment variables
	Config   []string               // Required config keys/paths
	Invocations []SkillInvocationPolicy // When skill can be invoked
	Tags     []string               // Arbitrary tags for grouping
	Examples []string               // Usage examples
	Priority int                    // Skill priority (higher = more important)
}

// Skill represents a loaded skill with parsed frontmatter
type Skill struct {
	Name                   string
	Description            string
	FilePath               string  // Absolute path to SKILL.md
	BaseDir                string  // Skill directory containing SKILL.md
	Source                 string  // Source label (e.g., "openclaw-workspace", "agents-skills-project")
	DisableModelInvocation bool    // If true, model should not invoke this skill directly
	UserInvocable          bool    // If true, user can invoke via slash command
	Content                string  // Markdown content after frontmatter
	Metadata               *SkillMetadata
	LoadedAt               time.Time
}

// SkillEntry represents a skill with runtime state
type SkillEntry struct {
	Skill    *Skill
	Eligible bool   // Whether skill passed all eligibility checks
	Reason   string // Why skill is/ isn't eligible (for debugging)
}

// SkillSource represents a skill source directory with its priority
type SkillSource struct {
	Path     string
	Priority int    // Higher = more important (for dedup)
	Label    string // Human-readable label (e.g., "openclaw-workspace")
}

// SkillPromptFormat defines the output format for skills in prompt
type SkillPromptFormat string

const (
	FormatFull    SkillPromptFormat = "full"    // name + description + path
	FormatCompact SkillPromptFormat = "compact" // name + location only
)

// SkillPrompt contains the built prompt and metadata
type SkillPrompt struct {
	Content   string
	Format    SkillPromptFormat
	Truncated bool
	Skipped   int    // Number of skills skipped due to size
	Warning   string
}

// SkillCommandSpec represents a slash command registration
type SkillCommandSpec struct {
	Name        string   // Command name (e.g., "bugfix")
	Description string   // Command description for help
	SkillName   string   // Associated skill name
	Aliases     []string // Alternative command names
}
