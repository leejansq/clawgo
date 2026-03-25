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
	"time"
)

// ShortTermMemory handles daily session memory storage
type ShortTermMemory struct {
	dir string
}

// NewShortTermMemory creates a new ShortTermMemory instance
func NewShortTermMemory(baseDir string) (*ShortTermMemory, error) {
	if baseDir == "" {
		baseDir = "/tmp/eino/memory"
	}

	dir := filepath.Join(baseDir, "memory")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	return &ShortTermMemory{
		dir: dir,
	}, nil
}

// Read reads short-term memory for a specific date
func (stm *ShortTermMemory) Read(ctx context.Context, date string) (string, error) {
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	filePath := filepath.Join(stm.dir, date+".md")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// Write writes content to short-term memory for a specific date
func (stm *ShortTermMemory) Write(ctx context.Context, date, content string) error {
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	filePath := filepath.Join(stm.dir, date+".md")

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	timestamp := time.Now().Format(time.RFC3339)
	entry := fmt.Sprintf("\n## %s\n\n%s\n", timestamp, content)

	_, err = f.WriteString(entry)
	return err
}

// ListDates lists all available short-term memory dates
func (stm *ShortTermMemory) ListDates(ctx context.Context) ([]string, error) {
	files, err := os.ReadDir(stm.dir)
	if err != nil {
		return nil, err
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

// Delete deletes short-term memory for a specific date
func (stm *ShortTermMemory) Delete(ctx context.Context, date string) error {
	filePath := filepath.Join(stm.dir, date+".md")
	return os.Remove(filePath)
}

// CleanOld removes short-term memory older than the specified days
func (stm *ShortTermMemory) CleanOld(ctx context.Context, days int) error {
	if days <= 0 {
		return nil
	}

	cutoff := time.Now().AddDate(0, 0, -days)

	files, err := os.ReadDir(stm.dir)
	if err != nil {
		return err
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		name := f.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}

		dateStr := strings.TrimSuffix(name, ".md")
		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}

		if date.Before(cutoff) {
			filePath := filepath.Join(stm.dir, name)
			os.Remove(filePath)
		}
	}

	return nil
}
