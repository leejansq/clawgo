/*
 * Copyright 2025 CloudWeGo Authors
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

package memory

import (
	"context"
	"fmt"
	"strings"
)

// ToolResult represents the result of a tool execution
type ToolResult struct {
	// Results for search operations
	Results []*SearchResult `json:"results,omitempty"`

	// Content for get/read operations
	Content string `json:"content,omitempty"`

	// Error message if any
	Error string `json:"error,omitempty"`

	// Disabled indicates if the tool is unavailable
	Disabled bool `json:"disabled,omitempty"`

	// Provider info
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`

	// Search mode info
	Mode string `json:"mode,omitempty"`
}

// MemorySearchParams represents the parameters for memory_search tool
type MemorySearchParams struct {
	Query      string `json:"query"`                // Search query (required)
	MaxResults *int   `json:"maxResults,omitempty"` // Maximum results
	MinScore   *float64 `json:"minScore,omitempty"` // Minimum score
}

// MemoryGetParams represents the parameters for memory_get tool
type MemoryGetParams struct {
	Path string `json:"path"` // Path to read (required)
	From *int   `json:"from,omitempty"` // Start line number
	Lines *int  `json:"lines,omitempty"` // Number of lines
}

// MemoryToolsConfig contains configuration for memory tools
type MemoryToolsConfig struct {
	// EnableHybridSearch enables hybrid search (vector + keyword)
	// Default: true
	EnableHybridSearch bool

	// VectorWeight weight for vector search (0-1)
	// Default: 0.7
	VectorWeight float64

	// EnableMMR enables MMR re-ranking
	// Default: false (opt-in like OpenClaw)
	EnableMMR bool

	// MMRLambda lambda parameter for MMR (0-1)
	// Default: 0.7
	MMRLambda float64

	// EnableTimeDecay enables time decay for short-term memory
	// Default: false (opt-in like OpenClaw)
	EnableTimeDecay bool

	// TimeDecayHalfLifeDays half-life for time decay in days
	// Default: 30
	TimeDecayHalfLifeDays float64

	// IncludeSessionMemory includes session transcripts
	// Default: false
	IncludeSessionMemory bool
}

// DefaultMemoryToolsConfig returns default configuration
func DefaultMemoryToolsConfig() *MemoryToolsConfig {
	return &MemoryToolsConfig{
		EnableHybridSearch:    true,
		VectorWeight:          0.7,
		EnableMMR:            false, // Opt-in like OpenClaw
		MMRLambda:            0.7,
		EnableTimeDecay:      false, // Opt-in like OpenClaw
		TimeDecayHalfLifeDays: 30,
		IncludeSessionMemory: false,
	}
}

// MemorySearchTool performs semantic search on memory
func MemorySearchTool(ctx context.Context, store MemoryStore, params MemorySearchParams, cfg *MemoryToolsConfig) (*ToolResult, error) {
	if cfg == nil {
		cfg = DefaultMemoryToolsConfig()
	}

	// Build search options
	opts := []SearchOption{
		WithSearchHybrid(cfg.EnableHybridSearch),
		WithSearchVectorWeight(cfg.VectorWeight),
		WithSearchMMR(cfg.EnableMMR),
		WithSearchMMRLambda(cfg.MMRLambda),
		WithSearchTimeDecay(cfg.EnableTimeDecay),
	}

	if params.MaxResults != nil && *params.MaxResults > 0 {
		opts = append(opts, WithSearchLimit(*params.MaxResults))
	}

	if params.MinScore != nil {
		opts = append(opts, WithSearchMinScore(*params.MinScore))
	}

	// Execute search
	results, err := store.Search(ctx, params.Query, opts...)
	if err != nil {
		return &ToolResult{
			Error:    fmt.Sprintf("memory search failed: %v", err),
			Disabled: false,
		}, nil
	}

	return &ToolResult{
		Results: results,
		Mode:    "hybrid",
	}, nil
}

// MemoryGetTool reads specific memory content
func MemoryGetTool(ctx context.Context, store MemoryStore, params MemoryGetParams) (*ToolResult, error) {
	path := params.Path

	// Determine if it's long-term or short-term memory
	var content string
	var err error

	if strings.HasPrefix(path, "memory/") || strings.Contains(path, ".md") {
		// Try to parse date from path like memory/2026-03-24.md
		date := parseDateFromPath(path)
		if date != "" {
			content, err = store.ReadShortTerm(ctx, date)
			if err != nil {
				return &ToolResult{Error: fmt.Sprintf("failed to read short-term memory: %v", err)}, nil
			}
		} else {
			// Try reading as long-term
			content, err = store.ReadLongTerm(ctx)
			if err != nil {
				return &ToolResult{Error: fmt.Sprintf("failed to read memory: %v", err)}, nil
			}
		}
	} else if path == "MEMORY.md" || path == "memory.md" {
		content, err = store.ReadLongTerm(ctx)
		if err != nil {
			return &ToolResult{Error: fmt.Sprintf("failed to read long-term memory: %v", err)}, nil
		}
	} else {
		// Assume it's a short-term memory path
		date := parseDateFromPath(path)
		if date != "" {
			content, err = store.ReadShortTerm(ctx, date)
			if err != nil {
				return &ToolResult{Error: fmt.Sprintf("failed to read short-term memory: %v", err)}, nil
			}
		} else {
			return &ToolResult{Error: "invalid memory path"}, nil
		}
	}

	// Apply line filtering if specified
	if params.From != nil || params.Lines != nil {
		lines := strings.Split(content, "\n")
		start := 0
		if params.From != nil && *params.From > 0 {
			start = *params.From - 1 // Convert to 0-indexed
		}
		end := len(lines)
		if params.Lines != nil && *params.Lines > 0 {
			end = start + *params.Lines
		}
		if end > len(lines) {
			end = len(lines)
		}
		if start < len(lines) {
			content = strings.Join(lines[start:end], "\n")
		} else {
			content = ""
		}
	}

	return &ToolResult{
		Content: content,
	}, nil
}

// parseDateFromPath extracts date from memory file path
func parseDateFromPath(path string) string {
	// Match patterns like: memory/2026-03-24.md, memory/2026-03-24, 2026-03-24
	parts := strings.Split(path, "/")
	for _, part := range parts {
		part = strings.TrimSuffix(part, ".md")
		if len(part) == 10 && part[4] == '-' && part[7] == '-' {
			return part
		}
	}
	return ""
}

// FormatSearchResults formats search results for display (OpenClaw pattern: path + lines)
func FormatSearchResults(results []*SearchResult) string {
	if len(results) == 0 {
		return "No relevant memories found."
	}

	var sb strings.Builder
	sb.WriteString("Relevant memories:\n\n")

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("--- Result %d (Score: %.2f) ---\n", i+1, r.Score))
		// OpenClaw citation format: path#line or path#line1-line2
		if r.StartLine == r.EndLine {
			sb.WriteString(fmt.Sprintf("Source: %s#L%d\n", r.Path, r.StartLine))
		} else {
			sb.WriteString(fmt.Sprintf("Source: %s#L%d-L%d\n", r.Path, r.StartLine, r.EndLine))
		}
		sb.WriteString(fmt.Sprintf("Type: %s\n", r.Type))
		if r.Date != "" {
			sb.WriteString(fmt.Sprintf("Date: %s\n", r.Date))
		}
		if len(r.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("Tags: %v\n", r.Tags))
		}
		sb.WriteString(fmt.Sprintf("Snippet:\n%s\n\n", r.Snippet))
	}

	return sb.String()
}
