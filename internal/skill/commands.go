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
	"strings"
)

// CommandRegistry manages skill command registrations
type CommandRegistry struct {
	commands map[string]*SkillCommandSpec
}

// NewCommandRegistry creates a new command registry
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		commands: make(map[string]*SkillCommandSpec),
	}
}

// Register registers a skill command
func (r *CommandRegistry) Register(spec *SkillCommandSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("command name cannot be empty")
	}

	name := normalizeCommandName(spec.Name)
	if _, exists := r.commands[name]; exists {
		return fmt.Errorf("command already registered: %s", spec.Name)
	}

	r.commands[name] = spec

	// Register aliases
	for _, alias := range spec.Aliases {
		aliasName := normalizeCommandName(alias)
		r.commands[aliasName] = spec
	}

	return nil
}

// Get returns a command spec by name
func (r *CommandRegistry) Get(name string) *SkillCommandSpec {
	return r.commands[normalizeCommandName(name)]
}

// List returns all registered commands
func (r *CommandRegistry) List() []*SkillCommandSpec {
	seen := make(map[*SkillCommandSpec]bool)
	var result []*SkillCommandSpec

	for _, spec := range r.commands {
		if !seen[spec] {
			seen[spec] = true
			result = append(result, spec)
		}
	}

	return result
}

// normalizeCommandName normalizes a command name
func normalizeCommandName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ToLower(name)
	name = strings.TrimPrefix(name, "/")
	return name
}

// BuildCommandSpecsFromSkills creates command specs from eligible skills
func BuildCommandSpecsFromSkills(entries []*SkillEntry) []*SkillCommandSpec {
	eligible := GetEligibleSkills(entries)

	var specs []*SkillCommandSpec
	for _, entry := range eligible {
		if entry.Skill.UserInvocable {
			specs = append(specs, &SkillCommandSpec{
				Name:        entry.Skill.Name,
				Description: entry.Skill.Description,
				SkillName:   entry.Skill.Name,
				Aliases:     []string{},
			})
		}
	}

	return specs
}

// ToolSpec represents a tool specification for skill commands
type ToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
}

// ToToolSpec converts a SkillCommandSpec to a ToolSpec
func (s *SkillCommandSpec) ToToolSpec() *ToolSpec {
	return &ToolSpec{
		Name:        "skill_" + s.Name,
		Description: s.Description,
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"skill_name": map[string]interface{}{
					"type":        "string",
					"description": fmt.Sprintf("The name of the skill to invoke: %s", s.Name),
				},
				"task": map[string]interface{}{
					"type":        "string",
					"description": "The task to execute with the skill",
				},
			},
			"required": []string{"task"},
		},
	}
}
