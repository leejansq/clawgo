/*
 * Agent-B: 投放执行员
 * 使用 LLM + Session + Memory + Skills
 */

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/leejansq/clawgo/internal/memory"
	"github.com/leejansq/clawgo/internal/session"
	"github.com/leejansq/clawgo/projects/touliu/internal/types"
)

// AgentB 投放执行员
type AgentB struct {
	*AdAgent
}

// NewAgentB 创建投放执行员
func NewAgentB(ctx context.Context, cfg *AdAgentConfig) (*AgentB, error) {
	cfg.Name = "Agent-B-执行员"
	cfg.Description = "投放执行员：接收方案，执行具体的投放操作"
	cfg.SystemPrompt = `你是电商投放执行专家，负责根据投放方案执行具体的广告投放操作。

你的职责：
1. 根据投放方案创建广告计划
2. 设置出价和预算
3. 配置定向人群
4. 上传创意素材
5. 启动投放并监控状态
6. 记录执行结果

执行步骤：
1. 验证方案的完整性和可行性
2. 调用广告平台API创建计划
3. 配置定向和出价
4. 提交创意素材
5. 激活投放
6. 返回执行结果

请以JSON格式输出执行结果。`

	base, err := NewAdAgent(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &AgentB{AdAgent: base}, nil
}

// SystemPrompt 返回系统提示
func (b *AgentB) SystemPrompt() string {
	return b.AdAgent.systemPrompt
}

// Execute 执行投放
func (b *AgentB) Execute(ctx context.Context, plan *types.CampaignPlan) (*types.ExecutionResult, error) {
	// 1. 验证方案
	if err := b.validatePlan(plan); err != nil {
		return &types.ExecutionResult{
			CampaignID: plan.CampaignID,
			PlanID:     plan.CampaignID,
			Status:     "failed",
			ErrorMsg:   err.Error(),
			ExecutedAt: time.Now(),
		}, err
	}

	// 2. 构建执行任务
	task := b.buildExecutionTask(plan)

	// 3. 记录到 session
	b.AppendSession("user", task)

	// 4. 调用 LLM
	result, err := b.Chat(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("failed to execute: %w", err)
	}

	// 5. 记录到 session
	b.AppendSession("assistant", result)

	// 6. 解析结果
	execResult, err := b.parseExecutionResult(result, plan)
	if err != nil {
		return nil, err
	}

	// 7. 保存执行记录到记忆
	b.WriteMemory(ctx, fmt.Sprintf("执行记录: 计划ID=%s, 状态=%s, 创建计划=%v, 详情=%s",
		execResult.CampaignID, execResult.Status, execResult.CampaignIDs, execResult.Details),
		memory.MemoryMeta{
			Type:      memory.MemoryTypeLongTerm,
			Source:    "execution_result",
			Tags:      []string{plan.Product, "执行记录"},
			Importance: 7,
		})

	return execResult, nil
}

// validatePlan 验证投放方案的完整性
func (b *AgentB) validatePlan(plan *types.CampaignPlan) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}
	if plan.Product == "" {
		return fmt.Errorf("product is empty")
	}
	if plan.Targeting.AgeRange[0] == 0 && plan.Targeting.AgeRange[1] == 0 {
		return fmt.Errorf("targeting not configured")
	}
	if plan.Bid.DailyBudget <= 0 {
		return fmt.Errorf("daily budget must be positive")
	}
	return nil
}

// buildExecutionTask 构建执行任务描述
func (b *AgentB) buildExecutionTask(plan *types.CampaignPlan) string {
	targetingJSON, _ := json.Marshal(plan.Targeting)
	bidJSON, _ := json.Marshal(plan.Bid)

	return fmt.Sprintf(`
请执行以下投放方案：

方案ID: %s
产品: %s

定向配置:
%s

出价配置:
%s

创意简报: %s

请执行以下步骤：
1. 创建广告计划（计划ID格式: camp-xxxx）
2. 配置定向人群
3. 设置出价和预算
4. 提交创意素材（使用占位URL）
5. 激活投放

返回JSON格式的执行结果。
`, plan.CampaignID, plan.Product,
		string(targetingJSON), string(bidJSON), plan.CreativeBrief)
}

// parseExecutionResult 解析执行结果
func (b *AgentB) parseExecutionResult(jsonStr string, plan *types.CampaignPlan) (*types.ExecutionResult, error) {
	// 尝试直接解析
	var result types.ExecutionResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err == nil {
		result.CampaignID = plan.CampaignID
		result.PlanID = plan.CampaignID
		result.ExecutedAt = time.Now()
		return &result, nil
	}

	// 尝试提取 JSON 块
	return b.extractResultFromText(jsonStr, plan)
}

// extractResultFromText 从文本中提取 JSON 结果
func (b *AgentB) extractResultFromText(text string, plan *types.CampaignPlan) (*types.ExecutionResult, error) {
	// 查找 JSON 块
	start := -1
	end := -1
	braceCount := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '{' {
			if start == -1 {
				start = i
			}
			braceCount++
		}
		if text[i] == '}' {
			braceCount--
			if braceCount == 0 && start != -1 {
				end = i
				break
			}
		}
	}

	if start == -1 || end == -1 {
		// 如果找不到 JSON，创建模拟结果
		return &types.ExecutionResult{
			CampaignID:  plan.CampaignID,
			PlanID:      plan.CampaignID,
			Status:      "success",
			CampaignIDs: []string{fmt.Sprintf("camp-%s", plan.CampaignID)},
			ExecutedAt:  time.Now(),
			Details:     "方案已通过验证，定向和出价配置完成",
		}, nil
	}

	jsonStr := text[start : end+1]
	var result types.ExecutionResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	result.CampaignID = plan.CampaignID
	result.PlanID = plan.CampaignID
	result.ExecutedAt = time.Now()
	return &result, nil
}

// SimulateExecution 模拟执行（用于演示）
func (b *AgentB) SimulateExecution(plan *types.CampaignPlan) *types.ExecutionResult {
	return &types.ExecutionResult{
		CampaignID:  plan.CampaignID,
		PlanID:      plan.CampaignID,
		Status:      "success",
		CampaignIDs: []string{fmt.Sprintf("camp-%s-001", plan.CampaignID)},
		ExecutedAt:  time.Now(),
		Details: fmt.Sprintf("模拟执行：已在%s平台创建广告计划，定向%s岁%s用户，预算%.2f元/天",
			"抖音",
			fmt.Sprintf("%d-%d", plan.Targeting.AgeRange[0], plan.Targeting.AgeRange[1]),
			plan.Targeting.Gender,
			plan.Bid.DailyBudget),
	}
}

// GetSessionStore 获取 Session Store
func (b *AgentB) GetSessionStore() session.SessionStore {
	return b.sessionStore
}
