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
)

// MemoryStore is the core interface for memory management
type MemoryStore interface {
	// Write writes content to memory
	Write(ctx context.Context, content string, meta MemoryMeta) error

	// Search searches memory for relevant content
	Search(ctx context.Context, query string, opts ...SearchOption) ([]*SearchResult, error)

	// ReadLongTerm reads all long-term memory content
	ReadLongTerm(ctx context.Context) (string, error)

	// WriteLongTerm writes content to long-term memory
	WriteLongTerm(ctx context.Context, content string) error

	// ReadShortTerm reads short-term memory for a specific date
	ReadShortTerm(ctx context.Context, date string) (string, error)

	// ListShortTermDates lists all available short-term memory dates
	ListShortTermDates(ctx context.Context) ([]string, error)

	// Close closes the memory store
	Close() error
}

// New creates a new MemoryStore with the given configuration
func New(ctx context.Context, cfg *Config) (MemoryStore, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if cfg.Embedder == nil {
		fmt.Println("xxxxxx------")
		embedder, err := DefaultZhipuEmbedder(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to create ZhipuEmbedder: %w", err)
		}
		cfg.Embedder = embedder
	}

	return newMemoryStore(ctx, cfg)
}
