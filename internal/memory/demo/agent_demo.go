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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/leejansq/clawgo/internal/memory"
)

// ============================================================================
// OpenClaw-aligned Memory Agent Demo
//
// This demo creates a simple agent-like experience with:
// - memory_search tool (semantic search on MEMORY.md + memory/*.md)
// - memory_get tool (read specific memory files)
// - Prompt section matching OpenClaw's memory recall guidance
//
// The memory tools here mirror exactly what OpenClaw provides:
//   - memory_search: Mandatory recall step before answering questions
//   - memory_get: Safe snippet read after memory_search
// ============================================================================

func main() {
	ctx := context.Background()

	// 1. Create memory store
	store, err := createMemoryStore(ctx)
	if err != nil {
		log.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	// 2. Write some test memory data
	writeTestMemory(ctx, store)

	// 3. Build system prompt (matching OpenClaw's memory-core plugin)
	systemPrompt := buildMemorySystemPrompt()

	// 4. Run demo
	fmt.Println("=== Memory Agent Demo (OpenClaw-aligned) ===")
	fmt.Println("\n--- System Prompt (Memory Recall Section) ---")
	fmt.Println(systemPrompt)

	// Demo conversation simulating agent behavior
	runMemoryRecallDemo(ctx, store)
}

// ============================================================================
// Memory Store Setup
// ============================================================================

func createMemoryStore(ctx context.Context) (memory.MemoryStore, error) {
	embedder := &mockEmbedder{}

	cfg := &memory.Config{
		BaseDir:      "/tmp/eino/memory_demo",
		VectorDBPath: "/tmp/eino/memory_demo/vector.db",
		Embedder:     embedder,
		ChunkSize:    512,
		ChunkOverlap: 50,
	}

	return memory.New(ctx, cfg)
}

func writeTestMemory(ctx context.Context, store memory.MemoryStore) {
	// Long-term memory (MEMORY.md)
	todayStr := time.Now().Format("2006-01-02")
	ltContent := fmt.Sprintf(`# User Preferences
- User prefers Go for backend development
- User likes clean, simple code
- User works on cloud-native applications
- User prefers afternoon meetings (after 2pm)
- User is allergic to shellfish

# Project Notes
- Main project is called "clawgo"
- Using eino framework for AI agent
- Vector storage with SQLite

# Important Dates
- Project kickoff: 2026-01-15
- Launch deadline: 2026-06-30
- Today: %s
`, todayStr)
	store.WriteLongTerm(ctx, ltContent)

	// Short-term memory (today's session - memory/YYYY-MM-DD.md)
	todayContent := `# Meeting - OAuth2 Implementation
- Discussed Google and GitHub OAuth2 integration
- JWT tokens for session management
- Need to implement refresh tokens
- Deadline: next Friday

# Code Review - PR #123
- Reviewed middleware implementation
- Requested changes to error handling
- Pair programming with Alice on caching layer

# Todo
- [ ] Complete OAuth2 flow
- [ ] Add unit tests for middleware
`
	store.Write(ctx, todayContent, memory.MemoryMeta{
		Type:       memory.MemoryTypeShortTerm,
		Date:       memory.GetTodayDate(),
		Source:     "meeting",
		Importance: 8,
		Tags:       []string{"feature", "auth"},
	})

	// Yesterday's short-term memory
	yesterdayContent := `# Pair Programming Session
- Worked on memory chunking algorithm
- Fixed bug in overlap calculation
- Test coverage improved to 85%
`
	store.Write(ctx, yesterdayContent, memory.MemoryMeta{
		Type:       memory.MemoryTypeShortTerm,
		Date:       memory.GetDateString(1),
		Source:     "coding",
		Importance: 6,
		Tags:       []string{"development"},
	})
}

// ============================================================================
// OpenClaw Memory Prompt Section
//
// This EXACTLY matches the prompt from extensions/memory-core/index.ts
// ============================================================================

func buildMemorySystemPrompt() string {
	// From openclaw/extensions/memory-core/index.ts:
	// buildPromptSection() returns these lines when both memory_search and memory_get are available
	return `## Memory Recall

Before answering anything about prior work, decisions, dates, people, preferences, or todos:
- Run memory_search on MEMORY.md + memory/*.md
- Then use memory_get to pull only the needed lines

If low confidence after search, say you checked.

Citations: include Source: <path#line> when it helps the user verify memory snippets.`
}

// ============================================================================
// Tool Definitions (EXACTLY matching OpenClaw's memory-tool.ts)
// ============================================================================

// ToolResult matches OpenClaw's jsonResult structure
type ToolResult struct {
	Results   interface{} `json:"results,omitempty"`
	Content   string       `json:"content,omitempty"`
	Path      string       `json:"path,omitempty"`
	Mode      string       `json:"mode,omitempty"`
	Provider  string       `json:"provider,omitempty"`
	Model     string       `json:"model,omitempty"`
	Disabled  bool         `json:"disabled,omitempty"`
	Unavailable bool       `json:"unavailable,omitempty"`
	Error     string       `json:"error,omitempty"`
	Warning   string       `json:"warning,omitempty"`
	Action    string       `json:"action,omitempty"`
}

// memory_search - matches OpenClaw's MemorySearchTool
//
// Mandatory recall step: semantically search MEMORY.md + memory/*.md before
// answering questions about prior work, decisions, dates, people, preferences, or todos.
// Returns top snippets with path + lines.
func memorySearch(query string, store memory.MemoryStore) *ToolResult {
	params := memory.MemorySearchParams{
		Query: query,
	}
	cfg := memory.DefaultMemoryToolsConfig()

	result, err := memory.MemorySearchTool(context.Background(), store, params, cfg)
	if err != nil {
		return &ToolResult{
			Error:    fmt.Sprintf("memory search failed: %v", err),
			Disabled: false,
		}
	}

	return &ToolResult{
		Results:  result.Results,
		Mode:     result.Mode,
		Provider: result.Provider,
		Model:    result.Model,
		Disabled: result.Disabled,
		Error:    result.Error,
	}
}

// memory_get - matches OpenClaw's MemoryGetTool
//
// Safe snippet read from MEMORY.md or memory/*.md with optional from/lines.
// Use after memory_search to pull only the needed lines.
func memoryGet(path string, from, lines *int, store memory.MemoryStore) *ToolResult {
	params := memory.MemoryGetParams{
		Path:  path,
		From:  from,
		Lines: lines,
	}

	result, err := memory.MemoryGetTool(context.Background(), store, params)
	if err != nil {
		return &ToolResult{
			Path:    path,
			Error:   fmt.Sprintf("memory get failed: %v", err),
			Disabled: true,
		}
	}

	return &ToolResult{
		Content: result.Content,
		Path:    path,
		Disabled: result.Disabled,
		Error:   result.Error,
	}
}

// ============================================================================
// Demo: Memory Recall Flow (simulating OpenClaw agent behavior)
// ============================================================================

func runMemoryRecallDemo(ctx context.Context, store memory.MemoryStore) {
	fmt.Println("\n--- Simulating Agent Memory Recall ---")
	fmt.Println("(This shows how an OpenClaw-aligned agent would use memory tools)\n")

	// Scenario 1: User asks about past work
	runScenario(ctx, store,
		"What did I work on yesterday?",
		"yesterday OR pair programming OR caching",
	)

	// Scenario 2: User asks about project details
	runScenario(ctx, store,
		"What's the main project I'm working on?",
		"project clawgo eino framework",
	)

	// Scenario 3: User asks about preferences
	runScenario(ctx, store,
		"What are my meeting preferences?",
		"meeting preferences afternoon",
	)

	// Scenario 4: User asks about deadlines
	runScenario(ctx, store,
		"What's my deadline for OAuth2?",
		"OAuth2 deadline Friday",
	)

	fmt.Println("\n=== Demo Complete ===")
}

func runScenario(ctx context.Context, store memory.MemoryStore, userQuestion, searchQuery string) {
	fmt.Printf("User: %s\n\n", userQuestion)

	// Step 1: Agent runs memory_search (like OpenClaw agent)
	fmt.Println("Agent thinking: I need to search memory for this...")
	fmt.Printf("  -> memory_search(query=%q)\n", searchQuery)

	result := memorySearch(searchQuery, store)
	jsonBytes, _ := json.MarshalIndent(result, "  ", "  ")
	fmt.Printf("  Result: %s\n\n", string(jsonBytes))

	// Step 2: If results found, use memory_get to get more details
	if results, ok := result.Results.([]*memory.SearchResult); ok && len(results) > 0 {
		// Take the first (highest scoring) result and use its path + lines
		first := results[0]
		fmt.Printf("Agent thinking: Let me get more details from %s (lines %d-%d)...\n",
			first.Path, first.StartLine, first.EndLine)

		// OpenClaw pattern: memory_get uses path + from + lines from search result
		from := first.StartLine
		lineCount := first.EndLine - first.StartLine + 1
		getResult := memoryGet(first.Path, &from, &lineCount, store)
		if getResult.Content != "" {
			// Show snippet as demo
			lines := splitLines(getResult.Content)
			fmt.Printf("  -> memory_get(%q, from=%d, lines=%d)\n", first.Path, from, lineCount)
			fmt.Printf("  Content:\n")
			for _, line := range lines {
				fmt.Printf("    %s\n", line)
			}
		}
	}

	// Step 3: Agent responds
	response := generateAgentResponse(userQuestion, result)
	fmt.Printf("\nAgent: %s\n", response)
	fmt.Println("\n" + stringsRepeat("-", 60) + "\n")
}

func generateAgentResponse(question string, searchResult *ToolResult) string {
	if searchResult.Disabled {
		return "I couldn't access your memory. The memory search is currently unavailable."
	}

	results, ok := searchResult.Results.([]*memory.SearchResult)
	if !ok || len(results) == 0 {
		return "I searched your memory but didn't find anything relevant. Could you provide more details?"
	}

	// Build response based on results (simplified)
	return fmt.Sprintf("Based on your memory, I found %d relevant entries. [In a real agent, this would be a detailed response citing the memory snippets with Source: paths]",
		len(results))
}

func splitLines(s string) []string {
	var lines []string
	for _, l := range s {
		if l == '\n' {
			lines = append(lines, "")
		} else {
			if len(lines) == 0 {
				lines = append(lines, "")
			}
			lines[len(lines)-1] += string(l)
		}
	}
	return lines
}

func stringsRepeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

// ============================================================================
// Mock Embedder (for demo without real embedding provider)
// ============================================================================

type mockEmbedder struct{}

func (m *mockEmbedder) EmbedStrings(ctx context.Context, texts []string) ([][]float64, error) {
	result := make([][]float64, len(texts))
	for i := range texts {
		result[i] = make([]float64, 1536)
		for j := range result[i] {
			result[i][j] = float64(i+j) * 0.1
		}
	}
	return result, nil
}

func intPtr(i int) *int {
	return &i
}
