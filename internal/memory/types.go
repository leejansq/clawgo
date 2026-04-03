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
	"time"
)

// MemoryType represents the type of memory
type MemoryType string

const (
	// MemoryTypeLongTerm represents long-term memory
	MemoryTypeLongTerm MemoryType = "long_term"
	// MemoryTypeShortTerm represents short-term memory (daily session)
	MemoryTypeShortTerm MemoryType = "short_term"
)

// MemoryMeta contains metadata for a memory entry
type MemoryMeta struct {
	Type       MemoryType // long_term / short_term
	Date       string     // YYYY-MM-DD (for short-term memory)
	Source     string     // Source identifier
	Importance int        // Importance score (0-10)
	Tags       []string   // Tags for categorization
	CreatedAt  time.Time  // Creation timestamp
}

// SearchResult represents a search result from memory (OpenClaw pattern: path + lines, content read from file)
type SearchResult struct {
	Path       string  // File path (e.g., "MEMORY.md" or "memory/2026-03-30.md")
	StartLine  int     // Start line number (1-indexed)
	EndLine    int     // End line number (1-indexed)
	Snippet    string  // Content snippet (read from file by memory_get)
	Score      float64 // Relevance score
	MemoryMeta         // Embedded metadata
	ChunkID    string  // Chunk identifier
}

// SearchOptions contains options for memory search
type SearchOptions struct {
	Limit                 int          // Maximum number of results
	MinScore              float64      // Minimum relevance score
	MemoryTypes           []MemoryType // Filter by memory types
	Dates                 []string     // Filter by dates (for short-term)
	Tags                  []string     // Filter by tags
	UseHybrid             bool         // Use hybrid search (vector + BM25)
	VectorWeight          float64      // Weight for vector search (0-1)
	UseMMR                bool         // Use MMR re-ranking
	MMRLambda             float64      // MMR lambda parameter (0-1)
	UseTimeDecay          bool         // Apply time decay
	TimeDecayFactor       float64      // Time decay factor per day
	TimeDecayHalfLifeDays float64      // Half-life for time decay in days (for display)
}

// SearchOption is a functional option for SearchOptions
type SearchOption func(*SearchOptions)

// DefaultSearchOptions returns default search options
func DefaultSearchOptions() *SearchOptions {
	return &SearchOptions{
		Limit:           5,
		MinScore:        0.0,
		MemoryTypes:     []MemoryType{MemoryTypeLongTerm, MemoryTypeShortTerm},
		UseHybrid:       true,
		VectorWeight:    0.7,
		UseMMR:          true,
		MMRLambda:       0.5,
		UseTimeDecay:    true,
		TimeDecayFactor: 0.95,
	}
}

// WithSearchLimit sets the maximum number of results
func WithSearchLimit(limit int) SearchOption {
	return func(o *SearchOptions) {
		o.Limit = limit
	}
}

// WithSearchMinScore sets the minimum relevance score
func WithSearchMinScore(score float64) SearchOption {
	return func(o *SearchOptions) {
		o.MinScore = score
	}
}

// WithSearchMemoryTypes filters by memory types
func WithSearchMemoryTypes(types ...MemoryType) SearchOption {
	return func(o *SearchOptions) {
		o.MemoryTypes = types
	}
}

// WithSearchDates filters by dates (for short-term memory)
func WithSearchDates(dates ...string) SearchOption {
	return func(o *SearchOptions) {
		o.Dates = dates
	}
}

// WithSearchTags filters by tags
func WithSearchTags(tags ...string) SearchOption {
	return func(o *SearchOptions) {
		o.Tags = tags
	}
}

// WithSearchHybrid enables/disables hybrid search
func WithSearchHybrid(use bool) SearchOption {
	return func(o *SearchOptions) {
		o.UseHybrid = use
	}
}

// WithSearchVectorWeight sets the vector search weight
func WithSearchVectorWeight(weight float64) SearchOption {
	return func(o *SearchOptions) {
		o.VectorWeight = weight
	}
}

// WithSearchMMR enables/disables MMR re-ranking
func WithSearchMMR(use bool) SearchOption {
	return func(o *SearchOptions) {
		o.UseMMR = use
	}
}

// WithSearchMMRLambda sets the MMR lambda parameter
func WithSearchMMRLambda(lambda float64) SearchOption {
	return func(o *SearchOptions) {
		o.MMRLambda = lambda
	}
}

// WithSearchTimeDecay enables/disables time decay
func WithSearchTimeDecay(use bool) SearchOption {
	return func(o *SearchOptions) {
		o.UseTimeDecay = use
	}
}

// WithSearchTimeDecayFactor sets the time decay factor
func WithSearchTimeDecayFactor(factor float64) SearchOption {
	return func(o *SearchOptions) {
		o.TimeDecayFactor = factor
	}
}

// Chunk represents a memory chunk stored in the vector database
type Chunk struct {
	ID      string
	Content string
	Vector  []float64
	MemoryMeta
}

// Config contains configuration for MemoryStore
type Config struct {
	// BaseDir is the base directory for storing memory files
	BaseDir string

	// VectorDBPath is the path to SQLite database for vector storage
	VectorDBPath string

	// Embedder is the embedding client for vector search
	// If nil, only BM25 search will be used
	Embedder Embedder

	// ChunkSize is the maximum size of each chunk
	ChunkSize int

	// ChunkOverlap is the overlap between chunks
	ChunkOverlap int
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		BaseDir:      "/tmp/eino/memory",
		VectorDBPath: "/tmp/eino/memory/vector.db",
		ChunkSize:    512,
		ChunkOverlap: 50,
	}
}

// Embedder is the interface for generating embeddings
// This can be implemented by wrapping eino's embedding.Embedder
type Embedder interface {
	// EmbedStrings generates embeddings for the given texts
	EmbedStrings(ctx context.Context, texts []string) ([][]float64, error)
}
