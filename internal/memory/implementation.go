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
	"crypto/sha256"
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

	// Chunk the content (meta is not used in chunking, per openclaw's chunkMarkdown)
	chunks := s.chunkContent(content)

	// Write chunks to vector store
	for _, chunk := range chunks {
		err := s.vectorStore.AddChunk(ctx, chunk.Text, ChunkMeta{
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

// lineEntry represents a line with its line number
type lineEntry struct {
	line   string
	lineNo int
}

// MemoryChunk represents a chunk with line information, matching openclaw's MemoryChunk
type MemoryChunk struct {
	StartLine int
	EndLine   int
	Text      string
	Hash      string
}

// chunkContent splits content into chunks, matching openclaw's chunkMarkdown logic
func (s *memoryStore) chunkContent(content string) []MemoryChunk {
	tokens := s.config.ChunkSize
	if tokens <= 0 {
		tokens = 512
	}

	overlap := s.config.ChunkOverlap
	if overlap <= 0 {
		overlap = 50
	}

	// maxChars = max(32, tokens * 4), 1 token ≈ 4 chars
	maxChars := tokens * 4
	if maxChars < 32 {
		maxChars = 32
	}

	// overlap in chars: overlap * 4
	overlapChars := overlap * 4
	if overlapChars < 0 {
		overlapChars = 0
	}

	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return []MemoryChunk{}
	}

	var chunks []MemoryChunk
	var current []lineEntry
	currentChars := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		firstEntry := current[0]
		lastEntry := current[len(current)-1]
		if firstEntry.line == "" && lastEntry.line == "" && len(current) == 1 {
			return
		}
		// Build text by joining lines with \n
		var text strings.Builder
		for i, entry := range current {
			if i > 0 {
				text.WriteString("\n")
			}
			text.WriteString(entry.line)
		}
		chunkText := text.String()
		chunks = append(chunks, MemoryChunk{
			StartLine: firstEntry.lineNo,
			EndLine:   lastEntry.lineNo,
			Text:      chunkText,
			Hash:      hashText(chunkText),
		})
	}

	carryOverlap := func() {
		if overlapChars <= 0 || len(current) == 0 {
			current = nil
			currentChars = 0
			return
		}
		// Keep lines from the end until we have enough overlap chars
		acc := 0
		kept := make([]lineEntry, 0)
		for i := len(current) - 1; i >= 0; i-- {
			entry := current[i]
			acc += len(entry.line) + 1 // +1 for newline
			kept = append([]lineEntry{entry}, kept...)
			if acc >= overlapChars {
				break
			}
		}
		current = kept
		currentChars = 0
		for _, entry := range current {
			currentChars += len(entry.line) + 1
		}
	}

	for i, line := range lines {
		lineNo := i + 1 // 1-indexed
		segments := make([]string, 0)

		if len(line) == 0 {
			segments = append(segments, "")
		} else {
			// Split long lines into segments of maxChars (matching openclaw's line slicing)
			for start := 0; start < len(line); start += maxChars {
				end := start + maxChars
				if end > len(line) {
					end = len(line)
				}
				segments = append(segments, line[start:end])
			}
		}

		for _, segment := range segments {
			lineSize := len(segment) + 1 // +1 for newline
			if currentChars+lineSize > maxChars && len(current) > 0 {
				flush()
				carryOverlap()
			}
			current = append(current, lineEntry{line: segment, lineNo: lineNo})
			currentChars += lineSize
		}
	}

	flush()
	return chunks
}

// hashText computes SHA256 hash of text, matching openclaw's hashText
func hashText(value string) string {
	h := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", h)
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
