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
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// VectorStore is a simple vector store using SQLite
type VectorStore struct {
	mu       sync.Mutex
	db       *sql.DB
	dbPath   string
	embedder Embedder
}

// NewVectorStore creates a new VectorStore
func NewVectorStore(ctx context.Context, dbPath string, embedder Embedder) (*VectorStore, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	vs := &VectorStore{
		db:       db,
		dbPath:   dbPath,
		embedder: embedder,
	}

	if err := vs.init(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return vs, nil
}

// init initializes the database schema
func (vs *VectorStore) init(ctx context.Context) error {
	// OpenClaw pattern: store path + line range in vector DB, content stays in file
	schema := `
	CREATE TABLE IF NOT EXISTS chunks (
		id TEXT PRIMARY KEY,
		path TEXT NOT NULL,
		start_line INTEGER NOT NULL,
		end_line INTEGER NOT NULL,
		vector BLOB,
		memory_type TEXT NOT NULL,
		date TEXT,
		source TEXT,
		importance INTEGER DEFAULT 5,
		tags TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_memory_type ON chunks(memory_type);
	CREATE INDEX IF NOT EXISTS idx_date ON chunks(date);
	CREATE INDEX IF NOT EXISTS idx_source ON chunks(source);
	CREATE INDEX IF NOT EXISTS idx_created_at ON chunks(created_at);
	`

	_, err := vs.db.Exec(schema)
	return err
}

// ChunkMeta contains metadata for a chunk
type ChunkMeta struct {
	ID         string     `json:"id"`
	Path       string     `json:"path"`        // ADD: file path (MEMORY.md or memory/YYYY-MM-DD.md)
	StartLine  int        `json:"start_line"`  // ADD: start line (1-indexed)
	EndLine    int        `json:"end_line"`    // ADD: end line (1-indexed)
	MemoryType MemoryType `json:"memory_type"`
	Date       string     `json:"date,omitempty"`
	Source     string     `json:"source,omitempty"`
	Importance int        `json:"importance"`
	Tags       []string   `json:"tags"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// AddChunk adds a chunk to the vector store (OpenClaw pattern: stores path + lines, embeds content for search)
func (vs *VectorStore) AddChunk(ctx context.Context, content string, meta ChunkMeta) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// Generate embedding if embedder is available
	var vectorBlob []byte
	if vs.embedder != nil {
		vectors, err := vs.embedder.EmbedStrings(ctx, []string{content})
		if err != nil {
			return fmt.Errorf("failed to generate embedding: %w", err)
		}
		if len(vectors) > 0 {
			vectorBlob, _ = json.Marshal(vectors[0])
		}
	}

	meta.ID = generateID()
	meta.CreatedAt = time.Now()
	meta.UpdatedAt = time.Now()

	tagsJSON, _ := json.Marshal(meta.Tags)

	// OpenClaw pattern: store path + line range, NOT content (content is in the file)
	_, err := vs.db.Exec(`
		INSERT INTO chunks (id, path, start_line, end_line, vector, memory_type, date, source, importance, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		meta.ID, meta.Path, meta.StartLine, meta.EndLine, vectorBlob, meta.MemoryType, meta.Date, meta.Source,
		meta.Importance, string(tagsJSON), meta.CreatedAt.Format(time.RFC3339), meta.UpdatedAt.Format(time.RFC3339),
	)

	return err
}

// Search searches for similar chunks using vector similarity
// OpenClaw pattern: returns path + line range, NOT content (content read from file via memory_get)
func (vs *VectorStore) Search(ctx context.Context, query string, limit int, filter Filter) ([]*VectorSearchResult, error) {
	if vs.embedder == nil {
		return nil, fmt.Errorf("embedder not configured")
	}

	// Generate query embedding
	vectors, err := vs.embedder.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("no embedding generated")
	}

	queryVector := vectors[0]

	// Build query - OpenClaw pattern: SELECT path, start_line, end_line (no content)
	queryBuilder := `
		SELECT id, path, start_line, end_line, vector, memory_type, date, source, importance, tags, created_at
		FROM chunks WHERE 1=1
	`

	args := []interface{}{}

	if len(filter.MemoryTypes) > 0 {
		placeholders := make([]string, len(filter.MemoryTypes))
		for i, t := range filter.MemoryTypes {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		queryBuilder += fmt.Sprintf(" AND memory_type IN (%s)", strings.Join(placeholders, ","))
	}

	if len(filter.Dates) > 0 {
		placeholders := make([]string, len(filter.Dates))
		for i, d := range filter.Dates {
			placeholders[i] = "?"
			args = append(args, d)
		}
		queryBuilder += fmt.Sprintf(" AND date IN (%s)", strings.Join(placeholders, ","))
	}

	if len(filter.Sources) > 0 {
		placeholders := make([]string, len(filter.Sources))
		for i, s := range filter.Sources {
			placeholders[i] = "?"
			args = append(args, s)
		}
		queryBuilder += fmt.Sprintf(" AND source IN (%s)", strings.Join(placeholders, ","))
	}

	rows, err := vs.db.Query(queryBuilder, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query chunks: %w", err)
	}
	defer rows.Close()

	var results []*VectorSearchResult
	for rows.Next() {
		var id, path, vectorBlob, memoryType, date, source, tagsStr, createdAtStr string
		var startLine, endLine, importance int

		err := rows.Scan(&id, &path, &startLine, &endLine, &vectorBlob, &memoryType, &date, &source, &importance, &tagsStr, &createdAtStr)
		if err != nil {
			continue
		}

		var vector []float64
		if vectorBlob != "" {
			json.Unmarshal([]byte(vectorBlob), &vector)
		}

		score := cosineSimilarity(queryVector, vector)

		// Apply minimum score filter
		if filter.MinScore > 0 && score < filter.MinScore {
			continue
		}

		var tags []string
		json.Unmarshal([]byte(tagsStr), &tags)

		createdAt, _ := time.Parse(time.RFC3339, createdAtStr)

		results = append(results, &VectorSearchResult{
			ID:    id,
			Score: score,
			Path:  path,
			StartLine: startLine,
			EndLine: endLine,
			Meta: ChunkMeta{
				ID:         id,
				Path:       path,
				StartLine:  startLine,
				EndLine:    endLine,
				MemoryType: MemoryType(memoryType),
				Date:       date,
				Source:     source,
				Importance: importance,
				Tags:       tags,
				CreatedAt:  createdAt,
			},
		})
	}

	// Limit results
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	// Sort by score descending
	sortByScore(results)

	return results, nil
}

// VectorSearchResult represents a vector search result (OpenClaw pattern: path + lines, no content)
type VectorSearchResult struct {
	ID        string
	Score     float64
	Path      string    // ADD: file path (content is read from file)
	StartLine int       // ADD: start line (1-indexed)
	EndLine   int       // ADD: end line (1-indexed)
	Meta      ChunkMeta
	// Content is NOT stored - read from file via memory_get
}

// Filter contains filter options for search
type Filter struct {
	MemoryTypes []MemoryType
	Dates       []string
	Sources     []string
	MinScore    float64
}

// BM25Search performs BM25 keyword search
// OpenClaw pattern: since content is not in DB, BM25 searches by path/line keywords
func (vs *VectorStore) BM25Search(ctx context.Context, query string, limit int, filter Filter) ([]*BM25SearchResult, error) {
	// Simple BM25-like implementation using SQL LIKE
	// OpenClaw pattern: since content is not stored, we store path keywords
	// For production, consider using a proper full-text search library

	keywords := strings.Fields(strings.ToLower(query))

	queryBuilder := `
		SELECT id, path, start_line, end_line, memory_type, date, source, importance, tags, created_at
		FROM chunks WHERE 1=1
	`

	args := []interface{}{}

	if len(filter.MemoryTypes) > 0 {
		placeholders := make([]string, len(filter.MemoryTypes))
		for i, t := range filter.MemoryTypes {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		queryBuilder += fmt.Sprintf(" AND memory_type IN (%s)", strings.Join(placeholders, ","))
	}

	if len(filter.Dates) > 0 {
		placeholders := make([]string, len(filter.Dates))
		for i, d := range filter.Dates {
			placeholders[i] = "?"
			args = append(args, d)
		}
		queryBuilder += fmt.Sprintf(" AND date IN (%s)", strings.Join(placeholders, ","))
	}

	// Add keyword search conditions for path
	if len(keywords) > 0 {
		conditions := make([]string, len(keywords))
		for i, kw := range keywords {
			conditions[i] = "LOWER(path) LIKE ?"
			args = append(args, "%"+kw+"%")
		}
		queryBuilder += " AND (" + strings.Join(conditions, " OR ") + ")"
	}

	rows, err := vs.db.Query(queryBuilder, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query chunks: %w", err)
	}
	defer rows.Close()

	var results []*BM25SearchResult
	for rows.Next() {
		var id, path, memoryType, date, source, tagsStr, createdAtStr string
		var startLine, endLine, importance int

		err := rows.Scan(&id, &path, &startLine, &endLine, &memoryType, &date, &source, &importance, &tagsStr, &createdAtStr)
		if err != nil {
			continue
		}

		// Calculate simple keyword match score on path
		score := calculateKeywordScore(path, keywords)

		var tags []string
		json.Unmarshal([]byte(tagsStr), &tags)

		createdAt, _ := time.Parse(time.RFC3339, createdAtStr)

		results = append(results, &BM25SearchResult{
			ID:    id,
			Score: score,
			Path:  path,
			StartLine: startLine,
			EndLine: endLine,
			Meta: ChunkMeta{
				ID:         id,
				Path:       path,
				StartLine:  startLine,
				EndLine:    endLine,
				MemoryType: MemoryType(memoryType),
				Date:       date,
				Source:     source,
				Importance: importance,
				Tags:       tags,
				CreatedAt:  createdAt,
			},
		})
	}

	// Sort by score descending
	sortBM25ByScore(results)

	// Limit results
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// BM25SearchResult represents a BM25 search result (OpenClaw pattern: path + lines, no content)
type BM25SearchResult struct {
	ID        string
	Score     float64
	Path      string    // ADD: file path
	StartLine int       // ADD: start line
	EndLine   int       // ADD: end line
	Meta      ChunkMeta
	// Content is NOT stored - read from file via memory_get
}

// DeleteChunksByDate deletes chunks for a specific date
func (vs *VectorStore) DeleteChunksByDate(ctx context.Context, date string) error {
	_, err := vs.db.Exec("DELETE FROM chunks WHERE date = ?", date)
	return err
}

// DeleteChunksByType deletes chunks for a specific memory type
func (vs *VectorStore) DeleteChunksByType(ctx context.Context, memType MemoryType) error {
	_, err := vs.db.Exec("DELETE FROM chunks WHERE memory_type = ?", string(memType))
	return err
}

// Close closes the vector store
func (vs *VectorStore) Close() error {
	return vs.db.Close()
}

// Helper functions

func generateID() string {
	return fmt.Sprintf("%d_%s", time.Now().UnixNano(), randomString(8))
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
		time.Sleep(time.Nanosecond)
	}
	return string(b)
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a) && i < len(b); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (normA * normB)
}

func calculateKeywordScore(content string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0
	}

	contentLower := strings.ToLower(content)
	matchCount := 0

	for _, kw := range keywords {
		if strings.Contains(contentLower, kw) {
			matchCount++
		}
	}

	// Score based on keyword coverage and content length
	return float64(matchCount) / float64(len(keywords))
}

func sortByScore(results []*VectorSearchResult) {
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

func sortBM25ByScore(results []*BM25SearchResult) {
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}
