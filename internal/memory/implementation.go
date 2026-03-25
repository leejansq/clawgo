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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// memoryStore is the implementation of MemoryStore
type memoryStore struct {
	mu         sync.Mutex
	config     *Config
	vectorStore *VectorStore

	// Long-term memory file
	longTermFile string

	// Short-term memory directory
	shortTermDir string
}

// newMemoryStore creates a new memoryStore
func newMemoryStore(ctx context.Context, cfg *Config) (*memoryStore, error) {
	// Create base directory
	if err := os.MkdirAll(cfg.BaseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	// Initialize vector store
	vectorStore, err := NewVectorStore(ctx, cfg.VectorDBPath, cfg.Embedder)
	if err != nil {
		return nil, fmt.Errorf("failed to create vector store: %w", err)
	}

	// Set up file paths
	longTermFile := filepath.Join(cfg.BaseDir, "MEMORY.md")
	shortTermDir := filepath.Join(cfg.BaseDir, "memory")

	if err := os.MkdirAll(shortTermDir, 0755); err != nil {
		vectorStore.Close()
		return nil, fmt.Errorf("failed to create short-term directory: %w", err)
	}

	return &memoryStore{
		config:       cfg,
		vectorStore:  vectorStore,
		longTermFile: longTermFile,
		shortTermDir: shortTermDir,
	}, nil
}

// Write writes content to memory
func (s *memoryStore) Write(ctx context.Context, content string, meta MemoryMeta) error {
	if meta.Type == "" {
		meta.Type = MemoryTypeShortTerm
	}

	// Set date for short-term memory
	if meta.Type == MemoryTypeShortTerm && meta.Date == "" {
		meta.Date = time.Now().Format("2006-01-02")
	}

	// Chunk the content
	chunks := s.chunkContent(content, meta)

	// Write chunks to vector store
	for _, chunk := range chunks {
		err := s.vectorStore.AddChunk(ctx, chunk, ChunkMeta{
			MemoryType: meta.Type,
			Date:       meta.Date,
			Source:     meta.Source,
			Importance: meta.Importance,
			Tags:       meta.Tags,
		})
		if err != nil {
			return fmt.Errorf("failed to add chunk: %w", err)
		}
	}

	// Also write to file for short-term memory
	if meta.Type == MemoryTypeShortTerm {
		return s.writeShortTermFile(ctx, meta.Date, content)
	}

	return nil
}

// chunkContent splits content into chunks
func (s *memoryStore) chunkContent(content string, meta MemoryMeta) []string {
	chunkSize := s.config.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 512
	}

	overlap := s.config.ChunkOverlap
	if overlap <= 0 {
		overlap = 50
	}

	// If content is small enough, return as single chunk
	if len(content) <= chunkSize {
		return []string{content}
	}

	// Simple chunking by sentences or paragraphs
	var chunks []string
	paragraphs := strings.Split(content, "\n\n")

	var currentChunk strings.Builder
	currentLen := 0

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		paraLen := len(para)

		// If single paragraph exceeds chunk size, split by sentences
		if paraLen > chunkSize {
			if currentLen > 0 {
				chunks = append(chunks, currentChunk.String())
				currentChunk.Reset()
				currentLen = 0
			}
			chunks = append(chunks, s.splitBySentences(para, chunkSize, overlap)...)
			continue
		}

		// Check if adding this paragraph would exceed chunk size
		if currentLen+paraLen+2 > chunkSize {
			if currentLen > 0 {
				chunks = append(chunks, currentChunk.String())
				// Keep overlap
				if overlap > 0 && currentLen > overlap {
					overlapText := currentChunk.String()[currentLen-overlap:]
					currentChunk.Reset()
					currentChunk.WriteString(overlapText)
					currentLen = len(overlapText)
				} else {
					currentChunk.Reset()
					currentLen = 0
				}
			}
		}

		if currentLen > 0 {
			currentChunk.WriteString("\n\n")
			currentLen += 2
		}
		currentChunk.WriteString(para)
		currentLen += paraLen
	}

	// Add remaining chunk
	if currentLen > 0 {
		chunks = append(chunks, currentChunk.String())
	}

	return chunks
}

// splitBySentences splits text by sentences
func (s *memoryStore) splitBySentences(text string, chunkSize, overlap int) []string {
	// Simple sentence splitting
	sentences := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})

	var chunks []string
	var current strings.Builder
	currentLen := 0

	for _, sent := range sentences {
		sent = strings.TrimSpace(sent)
		if sent == "" {
			continue
		}

		sentLen := len(sent) + 1 // +1 for period

		if currentLen+sentLen > chunkSize && currentLen > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
			currentLen = 0
		}

		if currentLen > 0 {
			current.WriteString(". ")
			currentLen += 2
		}
		current.WriteString(sent)
		currentLen += len(sent)
	}

	if currentLen > 0 {
		chunks = append(chunks, current.String()+".")
	}

	return chunks
}

// writeShortTermFile writes content to a short-term memory file
func (s *memoryStore) writeShortTermFile(ctx context.Context, date, content string) error {
	filePath := filepath.Join(s.shortTermDir, date+".md")

	// Append to existing file or create new
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	// Add timestamp header
	timestamp := time.Now().Format(time.RFC3339)
	entry := fmt.Sprintf("\n## %s\n\n%s\n", timestamp, content)

	_, err = f.WriteString(entry)
	return err
}

// Search searches memory for relevant content
func (s *memoryStore) Search(ctx context.Context, query string, opts ...SearchOption) ([]*SearchResult, error) {
	options := DefaultSearchOptions()
	for _, opt := range opts {
		opt(options)
	}

	// Build filter
	filter := Filter{
		MemoryTypes: options.MemoryTypes,
		Dates:       options.Dates,
		MinScore:    options.MinScore,
	}

	var results []*SearchResult

	if options.UseHybrid {
		// Hybrid search
		vectorResults, err := s.vectorStore.Search(ctx, query, options.Limit*2, filter)
		if err != nil {
			return nil, fmt.Errorf("vector search failed: %w", err)
		}

		bm25Results, err := s.vectorStore.BM25Search(ctx, query, options.Limit*2, filter)
		if err != nil {
			return nil, fmt.Errorf("BM25 search failed: %w", err)
		}

		// Merge results
		results = s.mergeResults(ctx, vectorResults, bm25Results, options)
	} else if s.vectorStore.embedder != nil {
		// Vector search only
		vectorResults, err := s.vectorStore.Search(ctx, query, options.Limit, filter)
		if err != nil {
			return nil, fmt.Errorf("vector search failed: %w", err)
		}

		for _, r := range vectorResults {
			results = append(results, &SearchResult{
				Content:    r.Content,
				Score:      r.Score,
				MemoryMeta: convertMeta(r.Meta),
				ChunkID:    r.ID,
			})
		}
	} else {
		// BM25 only
		bm25Results, err := s.vectorStore.BM25Search(ctx, query, options.Limit, filter)
		if err != nil {
			return nil, fmt.Errorf("BM25 search failed: %w", err)
		}

		for _, r := range bm25Results {
			results = append(results, &SearchResult{
				Content:    r.Content,
				Score:      r.Score,
				MemoryMeta: convertMeta(r.Meta),
				ChunkID:    r.ID,
			})
		}
	}

	// Apply MMR if enabled
	if options.UseMMR {
		results = s.applyMMR(ctx, results, options)
	}

	// Apply time decay if enabled
	if options.UseTimeDecay {
		results = s.applyTimeDecay(ctx, results, options)
	}

	// Limit final results
	if len(results) > options.Limit {
		results = results[:options.Limit]
	}

	return results, nil
}

// mergeResults merges vector and BM25 search results
func (s *memoryStore) mergeResults(ctx context.Context, vectorResults []*VectorSearchResult, bm25Results []*BM25SearchResult, opts *SearchOptions) []*SearchResult {
	vectorWeight := opts.VectorWeight
	bm25Weight := 1 - vectorWeight

	// Normalize scores
	maxVectorScore := 1.0
	maxBM25Score := 1.0
	if len(vectorResults) > 0 {
		maxVectorScore = vectorResults[0].Score
		if maxVectorScore == 0 {
			maxVectorScore = 1
		}
	}
	if len(bm25Results) > 0 {
		maxBM25Score = bm25Results[0].Score
		if maxBM25Score == 0 {
			maxBM25Score = 1
		}
	}

	// Merge using a map
	merged := make(map[string]*SearchResult)

	for _, r := range vectorResults {
		normalizedScore := r.Score / maxVectorScore
		weightedScore := normalizedScore * vectorWeight

		merged[r.ID] = &SearchResult{
			Content:    r.Content,
			Score:      weightedScore,
			MemoryMeta: convertMeta(r.Meta),
			ChunkID:    r.ID,
		}
	}

	for _, r := range bm25Results {
		normalizedScore := r.Score / maxBM25Score
		weightedScore := normalizedScore * bm25Weight

		if existing, ok := merged[r.ID]; ok {
			existing.Score += weightedScore
		} else {
			merged[r.ID] = &SearchResult{
				Content:    r.Content,
				Score:      weightedScore,
				MemoryMeta: convertMeta(r.Meta),
				ChunkID:    r.ID,
			}
		}
	}

	// Convert to slice and sort
	results := make([]*SearchResult, 0, len(merged))
	for _, r := range merged {
		results = append(results, r)
	}

	// Sort by score
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}

// applyMMR applies Maximum Marginal Relevance re-ranking
func (s *memoryStore) applyMMR(ctx context.Context, results []*SearchResult, opts *SearchOptions) []*SearchResult {
	if len(results) <= 1 {
		return results
	}

	lambda := opts.MMRLambda
	selected := make([]*SearchResult, 0, len(results))
	remaining := make([]*SearchResult, len(results))
	copy(remaining, results)

	// Select first result with highest score
	selected = append(selected, remaining[0])
	remaining = append(remaining[:0], remaining[1:]...)

	// Select remaining results
	for len(selected) < len(results) && len(remaining) > 0 {
		var bestIdx int
		var bestScore float64

		for i, r := range remaining {
			// Calculate MMR score
			relevance := r.Score
			diversity := s.maxSimilarity(r, selected)

			mmrScore := lambda*relevance - (1-lambda)*diversity

			if bestScore == 0 || mmrScore > bestScore {
				bestScore = mmrScore
				bestIdx = i
			}
		}

		selected = append(selected, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	return selected
}

// maxSimilarity calculates maximum similarity to selected results
func (s *memoryStore) maxSimilarity(result *SearchResult, selected []*SearchResult) float64 {
	maxSim := 0.0
	for _, s := range selected {
		// Simple word overlap similarity
		sim := textSimilarity(result.Content, s.Content)
		if sim > maxSim {
			maxSim = sim
		}
	}
	return maxSim
}

// textSimilarity calculates simple text similarity
func textSimilarity(a, b string) float64 {
	aWords := strings.Fields(strings.ToLower(a))
	bWords := strings.Fields(strings.ToLower(b))

	if len(aWords) == 0 || len(bWords) == 0 {
		return 0
	}

	aSet := make(map[string]int)
	bSet := make(map[string]int)

	for _, w := range aWords {
		aSet[w]++
	}
	for _, w := range bWords {
		bSet[w]++
	}

	intersection := 0
	for w, count := range aSet {
		if bCount, ok := bSet[w]; ok {
			if count < bCount {
				intersection += count
			} else {
				intersection += bCount
			}
		}
	}

	union := len(aWords) + len(bWords) - intersection
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

// applyTimeDecay applies time decay to scores
func (s *memoryStore) applyTimeDecay(ctx context.Context, results []*SearchResult, opts *SearchOptions) []*SearchResult {
	factor := opts.TimeDecayFactor
	if factor <= 0 || factor > 1 {
		factor = 0.95
	}

	now := time.Now()

	for _, r := range results {
		days := now.Sub(r.CreatedAt).Hours() / 24
		decay := 1.0
		for i := 0; i < int(days); i++ {
			decay *= factor
		}
		r.Score *= decay
	}

	// Re-sort by score
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}

// ReadLongTerm reads all long-term memory content
func (s *memoryStore) ReadLongTerm(ctx context.Context) (string, error) {
	data, err := os.ReadFile(s.longTermFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read long-term memory: %w", err)
	}
	return string(data), nil
}

// WriteLongTerm writes content to long-term memory
func (s *memoryStore) WriteLongTerm(ctx context.Context, content string) error {
	// Also write to vector store
	err := s.Write(ctx, content, MemoryMeta{
		Type:       MemoryTypeLongTerm,
		Importance: 10,
	})
	if err != nil {
		return err
	}

	// Write to file
	return os.WriteFile(s.longTermFile, []byte(content), 0644)
}

// ReadShortTerm reads short-term memory for a specific date
func (s *memoryStore) ReadShortTerm(ctx context.Context, date string) (string, error) {
	filePath := filepath.Join(s.shortTermDir, date+".md")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read short-term memory: %w", err)
	}
	return string(data), nil
}

// ListShortTermDates lists all available short-term memory dates
func (s *memoryStore) ListShortTermDates(ctx context.Context) ([]string, error) {
	files, err := os.ReadDir(s.shortTermDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read short-term directory: %w", err)
	}

	var dates []string
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		if strings.HasSuffix(name, ".md") {
			dates = append(dates, strings.TrimSuffix(name, ".md"))
		}
	}

	return dates, nil
}

// Close closes the memory store
func (s *memoryStore) Close() error {
	if s.vectorStore != nil {
		return s.vectorStore.Close()
	}
	return nil
}

// convertMeta converts ChunkMeta to MemoryMeta
func convertMeta(meta ChunkMeta) MemoryMeta {
	return MemoryMeta{
		Type:       meta.MemoryType,
		Date:       meta.Date,
		Source:     meta.Source,
		Importance: meta.Importance,
		Tags:       meta.Tags,
		CreatedAt:  meta.CreatedAt,
	}
}
