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
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// frontmatterRegex matches YAML frontmatter: --- ... ---
var frontmatterRegex = regexp.MustCompile(`(?s)^---\n(.+?)\n---\n(.*)`)

// SkillFrontmatter represents the YAML frontmatter structure
type SkillFrontmatter struct {
	Name                   string   `yaml:"name"`
	Description            string   `yaml:"description"`
	DisableModelInvocation bool     `yaml:"disable-model-invocation"`
	UserInvocable         bool     `yaml:"user-invocable"`
	OS                    []string `yaml:"os"`
	Bins                  []string `yaml:"bins"`
	Env                   []string `yaml:"env"`
	Config                []string `yaml:"config"`
	Invocations           []string `yaml:"invocations"`
	Tags                  []string `yaml:"tags"`
	Examples              []string `yaml:"examples"`
	Priority              int      `yaml:"priority"`
}

// ParseSkillFile reads a SKILL.md file and returns a Skill
func ParseSkillFile(filePath string) (*Skill, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill file: %w", err)
	}

	return ParseSkillContent(string(data), filePath)
}

// ParseSkillContent parses skill content (YAML frontmatter + markdown)
func ParseSkillContent(content string, filePath string) (*Skill, error) {
	matches := frontmatterRegex.FindStringSubmatch(content)
	if matches == nil {
		// No frontmatter - use directory name as name (OpenClaw pattern)
		dir := filepath.Dir(filePath)
		name := filepath.Base(dir)
		return &Skill{
			Name:        name,
			Description: "",
			FilePath:    filePath,
			BaseDir:     dir,
			Content:     content,
			LoadedAt:    time.Now(),
		}, nil
	}

	frontmatter := &SkillFrontmatter{}
	if err := yaml.Unmarshal([]byte(matches[1]), frontmatter); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Build metadata
	metadata := &SkillMetadata{
		OS:       frontmatter.OS,
		Bins:     frontmatter.Bins,
		Env:      frontmatter.Env,
		Config:   frontmatter.Config,
		Tags:     frontmatter.Tags,
		Examples: frontmatter.Examples,
		Priority: frontmatter.Priority,
	}

	// Parse invocation policies
	for _, inv := range frontmatter.Invocations {
		switch strings.ToLower(inv) {
		case "always":
			metadata.Invocations = append(metadata.Invocations, PolicyAlways)
		case "model-only":
			metadata.Invocations = append(metadata.Invocations, PolicyModelOnly)
		case "never":
			metadata.Invocations = append(metadata.Invocations, PolicyNever)
		}
	}

	// Determine name priority: frontmatter > filename
	name := frontmatter.Name
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}

	description := frontmatter.Description

	return &Skill{
		Name:                   name,
		Description:            description,
		FilePath:               filePath,
		BaseDir:                filepath.Dir(filePath),
		DisableModelInvocation: frontmatter.DisableModelInvocation,
		UserInvocable:          frontmatter.UserInvocable,
		Content:                strings.TrimSpace(matches[2]),
		Metadata:               metadata,
		LoadedAt:               time.Now(),
	}, nil
}

// ExtractFrontmatter extracts just the frontmatter from content (for validation)
func ExtractFrontmatter(content string) (string, error) {
	matches := frontmatterRegex.FindStringSubmatch(content)
	if matches == nil {
		return "", nil
	}
	return matches[1], nil
}

// ValidateFrontmatter validates a frontmatter YAML string
func ValidateFrontmatter(frontmatter string) error {
	var out SkillFrontmatter
	return yaml.Unmarshal([]byte(frontmatter), &out)
}
