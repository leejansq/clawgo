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
	"math"
	"strings"
	"time"
)

// HybridSearcher handles hybrid search combining vector and keyword search
type HybridSearcher struct {
	vectorStore *VectorStore
}

// NewHybridSearcher creates a new HybridSearcher
func NewHybridSearcher(vectorStore *VectorStore) *HybridSearcher {
	return &HybridSearcher{
		vectorStore: vectorStore,
	}
}

// Search performs hybrid search combining vector and BM25 results
func (hs *HybridSearcher) Search(ctx context.Context, query string, limit int, filter Filter, vectorWeight float64) ([]*SearchResult, error) {
	// Perform vector search
	vectorResults, err := hs.vectorStore.Search(ctx, query, limit*2, filter)
	if err != nil {
		return nil, err
	}

	// Perform BM25 search
	bm25Results, err := hs.vectorStore.BM25Search(ctx, query, limit*2, filter)
	if err != nil {
		return nil, err
	}

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

	bm25Weight := 1 - vectorWeight

	// Merge results
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

	// Sort by score descending
	sortBySearchResultScore(results)

	// Limit results
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// MMRReranker performs Maximum Marginal Relevance re-ranking
type MMRReranker struct {
	lambda float64
}

// NewMMRReranker creates a new MMRReranker
func NewMMRReranker(lambda float64) *MMRReranker {
	if lambda < 0 || lambda > 1 {
		lambda = 0.5
	}
	return &MMRReranker{lambda: lambda}
}

// Rerank re-ranks results using MMR
func (mmr *MMRReranker) Rerank(ctx context.Context, results []*SearchResult) []*SearchResult {
	if len(results) <= 1 {
		return results
	}

	lambda := mmr.lambda
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
			diversity := maxTextSimilarity(r, selected)

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

// TimeDecayer applies time decay to search results
type TimeDecayer struct {
	factor float64
}

// NewTimeDecayer creates a new TimeDecayer
func NewTimeDecayer(factor float64) *TimeDecayer {
	if factor <= 0 || factor > 1 {
		factor = 0.95
	}
	return &TimeDecayer{factor: factor}
}

// Apply applies time decay to results
func (td *TimeDecayer) Apply(ctx context.Context, results []*SearchResult) []*SearchResult {
	if len(results) == 0 {
		return results
	}

	now := time.Now()

	for _, r := range results {
		days := now.Sub(r.CreatedAt).Hours() / 24
		decay := math.Pow(td.factor, days)
		r.Score *= decay
	}

	// Re-sort by score
	sortBySearchResultScore(results)

	return results
}

// Helper functions

func sortBySearchResultScore(results []*SearchResult) {
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

func maxTextSimilarity(result *SearchResult, selected []*SearchResult) float64 {
	maxSim := 0.0
	for _, s := range selected {
		sim := textSimilarity(result.Content, s.Content)
		if sim > maxSim {
			maxSim = sim
		}
	}
	return maxSim
}

func textSimilarityScore(a, b string) float64 {
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
