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
	"os"
	"path/filepath"
)

// LongTermMemory handles persistent long-term memory storage
type LongTermMemory struct {
	filePath string
}

// NewLongTermMemory creates a new LongTermMemory instance
func NewLongTermMemory(baseDir string) (*LongTermMemory, error) {
	if baseDir == "" {
		baseDir = "/tmp/eino/memory"
	}

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}

	return &LongTermMemory{
		filePath: filepath.Join(baseDir, "MEMORY.md"),
	}, nil
}

// Read reads the long-term memory content
func (ltm *LongTermMemory) Read(ctx context.Context) (string, error) {
	data, err := os.ReadFile(ltm.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// Write writes content to long-term memory
func (ltm *LongTermMemory) Write(ctx context.Context, content string) error {
	return os.WriteFile(ltm.filePath, []byte(content), 0644)
}

// Append appends content to long-term memory
func (ltm *LongTermMemory) Append(ctx context.Context, content string) error {
	f, err := os.OpenFile(ltm.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(content)
	return err
}
