/*
 * Copyright 2026 CloudWeGo Authors
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

package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/leejansq/clawgo/pkg/types"
)

// ResultStore 结果存储 (用于异步通知)
type ResultStore struct {
	mu       sync.RWMutex
	results  map[string]*types.SubAgentInfo
	filePath string
}

// NewResultStore 创建结果存储
func NewResultStore(workspaceRoot string) *ResultStore {
	filePath := filepath.Join(workspaceRoot, ".subagent_results.jsonl")
	return &ResultStore{
		results:  make(map[string]*types.SubAgentInfo),
		filePath: filePath,
	}
}

// Save 保存结果到文件
func (r *ResultStore) Save(info *types.SubAgentInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results[info.SessionKey] = info

	// 追加到文件 (JSONL 格式)
	f, err := os.OpenFile(r.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		data, _ := json.Marshal(info)
		f.Write(data)
		f.WriteString("\n")
	}
}

// Get 获取结果
func (r *ResultStore) Get(sessionKey string) *types.SubAgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.results[sessionKey]
}

// GetAll 获取所有结果
func (r *ResultStore) GetAll() []*types.SubAgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*types.SubAgentInfo, 0, len(r.results))
	for _, info := range r.results {
		result = append(result, info)
	}
	return result
}
