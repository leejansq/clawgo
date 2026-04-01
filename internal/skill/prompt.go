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

const (
	// MaxSkillsPromptChars is the maximum characters for skills in prompt
	MaxSkillsPromptChars = 30000
	// CompactThreshold is when to switch from full to compact format
	CompactThreshold = 20000
)

// BuildSkillsPrompt builds the skills section for system prompt
func BuildSkillsPrompt(entries []*SkillEntry, format SkillPromptFormat) *SkillPrompt {
	if len(entries) == 0 {
		return &SkillPrompt{
			Content:   "",
			Format:    format,
			Truncated: false,
			Skipped:   0,
		}
	}

	eligible := GetEligibleSkills(entries)
	if len(eligible) == 0 {
		return &SkillPrompt{
			Content:   "",
			Format:    format,
			Truncated: false,
			Skipped:   0,
			Warning:   "No eligible skills available",
		}
	}

	var builder strings.Builder
	builder.WriteString("\n\n## Available Skills\n\n")

	totalChars := len(builder.String())

	for i, entry := range eligible {
		var skillStr string

		switch format {
		case FormatCompact:
			skillStr = buildCompactSkillString(entry)
		default:
			skillStr = buildFullSkillString(entry)
		}

		// Check if adding this skill would exceed budget
		if totalChars+len(skillStr) > MaxSkillsPromptChars {
			return &SkillPrompt{
				Content:   builder.String(),
				Format:    format,
				Truncated: true,
				Skipped:   len(eligible) - i,
				Warning:   fmt.Sprintf("Skills truncated: %d skills skipped due to size limit", len(eligible)-i),
			}
		}

		builder.WriteString(skillStr)
		builder.WriteString("\n")
		totalChars += len(skillStr) + 1

		// Check for compact threshold and switch format
		if format == FormatFull && totalChars > CompactThreshold && i < len(eligible)-1 {
			// Switch to compact format for remaining skills
			compactBuilder := strings.Builder{}
			compactBuilder.WriteString(builder.String())
			for j := i + 1; j < len(eligible); j++ {
				compactStr := buildCompactSkillString(eligible[j])
				compactBuilder.WriteString(compactStr)
				compactBuilder.WriteString("\n")
			}
			return &SkillPrompt{
				Content:   compactBuilder.String(),
				Format:    FormatCompact,
				Truncated: false,
				Skipped:   0,
				Warning:   "Skills prompt exceeded recommended size, switching to compact format",
			}
		}
	}

	return &SkillPrompt{
		Content:   builder.String(),
		Format:    format,
		Truncated: false,
		Skipped:   0,
	}
}

// buildFullSkillString builds a full skill entry string
func buildFullSkillString(entry *SkillEntry) string {
	var builder strings.Builder

	builder.WriteString("### ")
	builder.WriteString(entry.Skill.Name)
	builder.WriteString("\n")

	if entry.Skill.Description != "" {
		builder.WriteString(entry.Skill.Description)
		builder.WriteString("\n")
	}

	builder.WriteString("Location: ")
	builder.WriteString(entry.Skill.FilePath)
	builder.WriteString("\n")

	if entry.Skill.Source != "" {
		builder.WriteString("Source: ")
		builder.WriteString(entry.Skill.Source)
		builder.WriteString("\n")
	}

	if entry.Skill.Metadata != nil && len(entry.Skill.Metadata.Examples) > 0 {
		builder.WriteString("Examples:\n")
		for _, ex := range entry.Skill.Metadata.Examples {
			builder.WriteString("- ")
			builder.WriteString(ex)
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

// buildCompactSkillString builds a compact skill entry string
func buildCompactSkillString(entry *SkillEntry) string {
	return fmt.Sprintf("- %s (%s)",
		entry.Skill.Name,
		entry.Skill.FilePath)
}

// BuildSlashCommandList builds a list of available slash commands
func BuildSlashCommandList(entries []*SkillEntry) string {
	eligible := GetEligibleSkills(entries)

	var builder strings.Builder
	builder.WriteString("\n\n## Slash Commands\n\n")
	builder.WriteString("Available commands:\n")

	for _, entry := range eligible {
		if entry.Skill.UserInvocable {
			builder.WriteString("/")
			builder.WriteString(entry.Skill.Name)
			if entry.Skill.Description != "" {
				builder.WriteString(" - ")
				builder.WriteString(entry.Skill.Description)
			}
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

// FormatSkillForModel formats a skill for direct model invocation
func FormatSkillForModel(entry *SkillEntry) string {
	var builder strings.Builder

	builder.WriteString("## Skill: ")
	builder.WriteString(entry.Skill.Name)
	builder.WriteString("\n\n")

	if entry.Skill.Description != "" {
		builder.WriteString(entry.Skill.Description)
		builder.WriteString("\n\n")
	}

	// Include content if available
	if entry.Skill.Content != "" {
		builder.WriteString("### Instructions\n")
		builder.WriteString(entry.Skill.Content)
		builder.WriteString("\n")
	}

	return builder.String()
}

// GetSkillByName finds a skill by name from a list of entries
func GetSkillByName(entries []*SkillEntry, name string) *SkillEntry {
	for _, entry := range entries {
		if entry.Skill.Name == name {
			return entry
		}
	}
	return nil
}
