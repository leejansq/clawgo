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
	"fmt"
	"log"

	"github.com/leejansq/clawgo/internal/memory"
)

func main() {
	ctx := context.Background()

	// Create embedder (mock for demo)
	embedder := &mockEmbedder{}

	// Create memory store
	store, err := memory.New(ctx, &memory.Config{
		BaseDir:      "/tmp/eino/memory_demo",
		VectorDBPath: "/tmp/eino/memory_demo/vector.db",
		Embedder:     embedder,
		ChunkSize:    512,
		ChunkOverlap: 50,
	})
	if err != nil {
		log.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	// Demo: Write test data
	fmt.Println("=== Writing test data ===")
	writeTestData(ctx, store)

	// Demo: Using memory_search tool (like OpenClaw)
	fmt.Println("\n=== Demo: memory_search Tool ===")
	testMemorySearchTool(ctx, store)

	// Demo: Using memory_get tool
	fmt.Println("\n=== Demo: memory_get Tool ===")
	testMemoryGetTool(ctx, store)

	// Demo: Tool configuration (OpenClaw style)
	fmt.Println("\n=== Demo: Tool Configuration ===")
	testToolConfig(ctx, store)

	fmt.Println("\n=== Demo Complete ===")
}

func writeTestData(ctx context.Context, store memory.MemoryStore) {
	// Write long-term memory
	ltContent := `# User Preferences
- User prefers Go for backend development
- User likes clean, simple code
- User works on cloud-native applications
- User prefers afternoon meetings (after 2pm)
- User is allergic to shellfish
`
	store.WriteLongTerm(ctx, ltContent)

	// Write today's short-term memory
	today := memory.GetTodayDate()
	todayContent := `User asked to implement OAuth2 authentication.
- Discussed Google and GitHub OAuth2 integration
- JWT tokens for session management
- Deadline: next Friday
`
	store.Write(ctx, todayContent, memory.MemoryMeta{
		Type:       memory.MemoryTypeShortTerm,
		Date:       today,
		Source:     "meeting",
		Importance: 8,
		Tags:       []string{"feature", "auth"},
	})

	// Write yesterday's short-term memory
	yesterday := memory.GetDateString(1)
	yesterdayContent := `Code review session for PR #123
- Reviewed middleware implementation
- Pair programming with Alice on caching
`
	store.Write(ctx, yesterdayContent, memory.MemoryMeta{
		Type:       memory.MemoryTypeShortTerm,
		Date:       yesterday,
		Source:     "coding",
		Importance: 6,
		Tags:       []string{"review", "pair-programming"},
	})
}

// testMemorySearchTool demonstrates the memory_search tool
func testMemorySearchTool(ctx context.Context, store memory.MemoryStore) {
	// Default config (hybrid enabled, MMR/time decay disabled like OpenClaw)
	cfg := memory.DefaultMemoryToolsConfig()

	// Search with default config
	params := memory.MemorySearchParams{
		Query: "OAuth2 JWT authentication",
	}

	result, err := memory.MemorySearchTool(ctx, store, params, cfg)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	if result.Error != "" {
		fmt.Printf("Error: %s\n", result.Error)
		return
	}

	fmt.Printf("Mode: %s\n", result.Mode)
	fmt.Printf("Results found: %d\n", len(result.Results))

	// Format results
	fmt.Println(memory.FormatSearchResults(result.Results))
}

// testMemoryGetTool demonstrates the memory_get tool
func testMemoryGetTool(ctx context.Context, store memory.MemoryStore) {
	// Get today's memory
	params := memory.MemoryGetParams{
		Path:  "memory/" + memory.GetTodayDate() + ".md",
		From:  intPtr(1),
		Lines: intPtr(10),
	}

	result, err := memory.MemoryGetTool(ctx, store, params)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	if result.Error != "" {
		fmt.Printf("Error: %s\n", result.Error)
		return
	}

	fmt.Printf("Content:\n%s\n", result.Content)

	// Get long-term memory
	params2 := memory.MemoryGetParams{
		Path: "MEMORY.md",
	}
	result2, _ := memory.MemoryGetTool(ctx, store, params2)
	fmt.Printf("\nLong-term memory:\n%s\n", result2.Content)
}

// testToolConfig demonstrates different tool configurations
func testToolConfig(ctx context.Context, store memory.MemoryStore) {
	// Config 1: Default (like OpenClaw - hybrid only)
	fmt.Println("--- Config 1: Default (Hybrid) ---")
	cfg1 := memory.DefaultMemoryToolsConfig()
	fmt.Printf("EnableHybrid: %v, VectorWeight: %.2f\n", cfg1.EnableHybridSearch, cfg1.VectorWeight)
	fmt.Printf("EnableMMR: %v, EnableTimeDecay: %v\n", cfg1.EnableMMR, cfg1.EnableTimeDecay)

	// Config 2: With MMR enabled
	fmt.Println("\n--- Config 2: Hybrid + MMR ---")
	cfg2 := &memory.MemoryToolsConfig{
		EnableHybridSearch: true,
		VectorWeight:       0.7,
		EnableMMR:          true,
		MMRLambda:          0.7,
		EnableTimeDecay:    false,
	}
	fmt.Printf("EnableHybrid: %v, EnableMMR: %v, MMRLambda: %.2f\n", cfg2.EnableHybridSearch, cfg2.EnableMMR, cfg2.MMRLambda)

	// Search with MMR
	params := memory.MemorySearchParams{
		Query:      "authentication implementation",
		MaxResults: intPtr(3),
	}
	result, _ := memory.MemorySearchTool(ctx, store, params, cfg2)
	fmt.Printf("Results with MMR: %d\n", len(result.Results))

	// Config 3: With Time Decay enabled
	fmt.Println("\n--- Config 3: Hybrid + Time Decay ---")
	cfg3 := &memory.MemoryToolsConfig{
		EnableHybridSearch:    true,
		VectorWeight:          0.7,
		EnableMMR:             false,
		EnableTimeDecay:       true,
		TimeDecayHalfLifeDays: 30,
	}
	fmt.Printf("EnableHybrid: %v, EnableTimeDecay: %v, HalfLifeDays: %.0f\n", cfg3.EnableHybridSearch, cfg3.EnableTimeDecay, cfg3.TimeDecayHalfLifeDays)

	result3, _ := memory.MemorySearchTool(ctx, store, params, cfg3)
	fmt.Printf("Results with Time Decay: %d\n", len(result3.Results))

	// Config 4: BM25 only (no vector)
	fmt.Println("\n--- Config 4: BM25 Only ---")
	cfg4 := &memory.MemoryToolsConfig{
		EnableHybridSearch: false,
	}
	fmt.Printf("EnableHybrid: %v (BM25 only)\n", cfg4.EnableHybridSearch)

	result4, _ := memory.MemorySearchTool(ctx, store, params, cfg4)
	fmt.Printf("Results with BM25: %d\n", len(result4.Results))
}

func intPtr(i int) *int {
	return &i
}

// mockEmbedder is a simple mock embedder for demo purposes
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
