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

// Search is a convenience function to search memory
func Search(ctx context.Context, store MemoryStore, query string, opts ...SearchOption) ([]*SearchResult, error) {
	return store.Search(ctx, query, opts...)
}

// SearchLongTerm searches only long-term memory
func SearchLongTerm(ctx context.Context, store MemoryStore, query string, opts ...SearchOption) ([]*SearchResult, error) {
	options := append(opts, WithSearchMemoryTypes(MemoryTypeLongTerm))
	return store.Search(ctx, query, options...)
}

// SearchShortTerm searches only short-term memory
func SearchShortTerm(ctx context.Context, store MemoryStore, query string, opts ...SearchOption) ([]*SearchResult, error) {
	options := append(opts, WithSearchMemoryTypes(MemoryTypeShortTerm))
	return store.Search(ctx, query, options...)
}

// SearchToday searches today's short-term memory
func SearchToday(ctx context.Context, store MemoryStore, query string, opts ...SearchOption) ([]*SearchResult, error) {
	today := GetTodayDate()
	options := append(opts, WithSearchMemoryTypes(MemoryTypeShortTerm), WithSearchDates(today))
	return store.Search(ctx, query, options...)
}

// GetTodayDate returns today's date in YYYY-MM-DD format
func GetTodayDate() string {
	return time.Now().Format("2006-01-02")
}

// GetDateString returns the date string for the given number of days ago
func GetDateString(daysAgo int) string {
	d := time.Now().AddDate(0, 0, -daysAgo)
	return d.Format("2006-01-02")
}
