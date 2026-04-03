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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// ZhipuEmbedderConfig is the configuration for ZhipuEmbedder
type ZhipuEmbedderConfig struct {
	APIKey     string // Zhipu API Key
	Model      string // Embedding model, default "embedding-3"
	Dimensions int    // Embedding dimensions, default 1024
	Timeout    int    // Timeout in seconds, default 30
}

// ZhipuEmbedder implements Embedder interface using Zhipu AI's embedding API
type ZhipuEmbedder struct {
	config   ZhipuEmbedderConfig
	client   *http.Client
	endpoint string
}

// NewZhipuEmbedder creates a new ZhipuEmbedder
func NewZhipuEmbedder(ctx context.Context, cfg *ZhipuEmbedderConfig) (*ZhipuEmbedder, error) {
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("ZHIPU_API_KEY")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("Zhipu API key is required")
	}
	if cfg.Model == "" {
		cfg.Model = "embedding-3"
	}
	if cfg.Dimensions <= 0 {
		cfg.Dimensions = 1024
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30
	}

	return &ZhipuEmbedder{
		config: *cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		endpoint: "https://open.bigmodel.cn/api/paas/v4/embeddings",
	}, nil
}

// EmbedStrings generates embeddings for the given texts
func (e *ZhipuEmbedder) EmbedStrings(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := map[string]interface{}{
		"model":      e.config.Model,
		"input":      texts,
		"dimensions": e.config.Dimensions,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+e.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Zhipu API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Zhipu API returned status %d", resp.StatusCode)
	}

	var result ZhipuEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no embedding data returned")
	}

	// Sort by index to maintain order
	embeddings := make([][]float64, len(result.Data))
	for _, d := range result.Data {
		if d.Index >= 0 && d.Index < len(embeddings) {
			embeddings[d.Index] = d.Embedding
		}
	}

	return embeddings, nil
}

// ZhipuEmbeddingResponse is the response from Zhipu embedding API
type ZhipuEmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// DefaultZhipuEmbedder creates a ZhipuEmbedder using environment variables
// ENV: ZHIPU_API_KEY, ZHIPU_MODEL (optional, defaults to embedding-3)
func DefaultZhipuEmbedder(ctx context.Context) (Embedder, error) {
	model := os.Getenv("ZHIPU_MODEL")
	return NewZhipuEmbedder(ctx, &ZhipuEmbedderConfig{
		Model: model,
	})
}

// IsZhipuEmbedderAvailable checks if Zhipu embedder can be created
func IsZhipuEmbedderAvailable() bool {
	return os.Getenv("ZHIPU_API_KEY") != ""
}

// StringEmbedder is a simple embedder that returns fingerprint hashes as pseudo-vectors
// Used as fallback when no real embedder is configured
type StringEmbedder struct{}

// NewStringEmbedder creates a StringEmbedder (fallback when no embedder is available)
func NewStringEmbedder() *StringEmbedder {
	return &StringEmbedder{}
}

// EmbedStrings returns pseudo-embeddings based on string hashes
func (e *StringEmbedder) EmbedStrings(ctx context.Context, texts []string) ([][]float64, error) {
	embeddings := make([][]float64, len(texts))
	for i, text := range texts {
		// Generate a simple hash-based pseudo-vector
		h := hashStringToFloats(text)
		embeddings[i] = h
	}
	return embeddings, nil
}

// hashStringToFloats converts a string to a pseudo-embedding vector
func hashStringToFloats(s string) []float64 {
	// Simple hash-based pseudo-vector (for fallback only)
	vec := make([]float64, 8)
	h := hashStrings(s)
	for i := range vec {
		vec[i] = float64((h>>(i*8))&0xFF) / 255.0
	}
	return vec
}

func hashStrings(s string) uint64 {
	var h uint64
	for i, c := range s {
		h ^= uint64(c) + 0x9e3779b97f4a7c15 + (h << 6) + (h >> 2)
		h *= uint64(i+1) * 0x9e3779b97f4a7c15
	}
	return h
}

// DetectAndCreateEmbedder auto-detects and creates an appropriate embedder
// Priority: Zhipu (if API key available) > String (fallback)
func DetectAndCreateEmbedder(ctx context.Context) (Embedder, error) {
	// Try Zhipu first
	if IsZhipuEmbedderAvailable() {
		return DefaultZhipuEmbedder(ctx)
	}

	// Check for OpenAI
	if os.Getenv("OPENAI_API_KEY") != "" {
		// Could add OpenAI embedder here if needed
		// For now, fall through to string embedder
	}

	// Fallback to string embedder (won't work well but won't crash)
	return NewStringEmbedder(), nil
}

// ZhipuEmbeddingRequest is the request body for Zhipu embedding API
type ZhipuEmbeddingRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
	Encode     string   `json:"encode,omitempty"`
}

// trimEmbedder creates a wrapper that trims input text before embedding
type TrimEmbedder struct {
	inner Embedder
}

// NewTrimEmbedder creates an embedder that trims input text
func NewTrimEmbedder(inner Embedder) Embedder {
	return &TrimEmbedder{inner: inner}
}

// EmbedStrings trims whitespace before calling the inner embedder
func (e *TrimEmbedder) EmbedStrings(ctx context.Context, texts []string) ([][]float64, error) {
	trimmed := make([]string, len(texts))
	for i, t := range texts {
		trimmed[i] = strings.TrimSpace(t)
	}
	return e.inner.EmbedStrings(ctx, trimmed)
}
