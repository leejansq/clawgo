/*
 * Agent-C: 效果评估与经验沉淀员
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

// AgentC 效果评估与经验沉淀员
type AgentC struct {
	*AdAgent
}

// NewAgentC 创建效果评估与经验沉淀员
func NewAgentC(ctx context.Context, cfg *AdAgentConfig) (*AgentC, error) {
	cfg.Name = "Agent-C-评估员"
	cfg.Description = "效果评估与经验沉淀员：监控投放数据，评估ROI，总结经验并更新知识库"
	cfg.SystemPrompt = `你是电商投放效果评估专家，负责评估投放效果并沉淀经验。

你的职责：
1. 收集和监控投放数据
2. 计算关键指标（ROI、CPA、CTR等）
3. 评估投放效果
4. 总结经验教训
5. 更新知识库

评估维度：
1. 效果达成情况
   - ROI达成率
   - 转化成本
   - 点击率
2. 经验教训
   - 成功因素
   - 失败原因
   - 优化方向
3. 知识沉淀
   - 更新经验库
   - 优化投放策略

请以JSON格式输出评估结果。`

	base, err := NewAdAgent(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &AgentC{AdAgent: base}, nil
}

// SystemPrompt 返回系统提示
func (c *AgentC) SystemPrompt() string {
	return c.AdAgent.systemPrompt
}

// Evaluate 评估投放效果
func (c *AgentC) Evaluate(ctx context.Context, plan *types.CampaignPlan, execution *types.ExecutionResult, perfData *types.PerformanceData) (*types.EvaluationResult, error) {
	// 1. 获取历史经验
	historicalResults, _ := c.SearchMemory(ctx, fmt.Sprintf("%s %s 效果评估", plan.Product, "抖音"), 5)

	// 2. 构建评估任务
	task := c.buildEvaluationTask(plan, execution, perfData, historicalResults)

	// 3. 记录到 session
	c.AppendSession("user", task)

	// 4. 调用 LLM
	result, err := c.Chat(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("failed to execute evaluation: %w", err)
	}

	// 5. 记录到 session
	c.AppendSession("assistant", result)

	// 6. 解析结果
	evalResult, err := c.parseEvaluationResult(result, plan, perfData)
	if err != nil {
		return nil, err
	}

	// 7. 沉淀经验到记忆
	c.沉淀经验(ctx, plan, evalResult)

	return evalResult, nil
}

// buildEvaluationTask 构建评估任务描述
func (c *AgentC) buildEvaluationTask(plan *types.CampaignPlan, execution *types.ExecutionResult, perfData *types.PerformanceData, historicalResults []*memory.SearchResult) string {
	perfJSON, _ := json.Marshal(perfData)
	planJSON, _ := json.Marshal(plan)

	return fmt.Sprintf(`
请评估以下投放效果：

方案信息:
%s

执行结果:
- 状态: %s
- 创建计划: %v
- 执行时间: %s

投放数据:
%s

历史评估结果:
%s

请分析以上数据，输出评估结果JSON，包括：
1. analysis: 效果分析
2. roiAchieved: 实际ROI
3. roiExpected: 预期ROI
4. achievementRate: ROI达成率
5. lessonsLearned: 经验教训
6. recommendations: 优化建议
7. status: 效果等级（excellent/good/needs_improvement/poor）
`, string(planJSON), execution.Status, execution.CampaignIDs,
		execution.ExecutedAt.Format(time.RFC3339), string(perfJSON),
		c.formatHistoricalResults(historicalResults))
}

// parseEvaluationResult 解析评估结果
func (c *AgentC) parseEvaluationResult(jsonStr string, plan *types.CampaignPlan, perfData *types.PerformanceData) (*types.EvaluationResult, error) {
	// 尝试直接解析
	var result types.EvaluationResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err == nil {
		result.CampaignID = plan.CampaignID
		result.PerformanceData = *perfData
		c.calculateAchievementRate(&result, plan)
		return &result, nil
	}

	// 尝试提取 JSON 块
	return c.extractResultFromText(jsonStr, plan, perfData)
}

// extractResultFromText 从文本中提取 JSON 结果
func (c *AgentC) extractResultFromText(text string, plan *types.CampaignPlan, perfData *types.PerformanceData) (*types.EvaluationResult, error) {
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
		result := c.simulateEvaluation(plan, perfData)
		return &result, nil
	}

	jsonStr := text[start : end+1]
	var result types.EvaluationResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	result.CampaignID = plan.CampaignID
	result.PerformanceData = *perfData
	c.calculateAchievementRate(&result, plan)
	return &result, nil
}

// calculateAchievementRate 计算 ROI 达成率
func (c *AgentC) calculateAchievementRate(result *types.EvaluationResult, plan *types.CampaignPlan) {
	if plan.ROIExpectation > 0 && result.PerformanceData.ROI > 0 {
		result.ROIExpected = plan.ROIExpectation
		result.ROIAchieved = result.PerformanceData.ROI
		result.AchievementRate = result.PerformanceData.ROI / plan.ROIExpectation * 100
	}
}

// simulateEvaluation 模拟评估（用于演示）
func (c *AgentC) simulateEvaluation(plan *types.CampaignPlan, perfData *types.PerformanceData) types.EvaluationResult {
	status := "good"
	if perfData.ROI >= plan.ROIExpectation*1.2 {
		status = "excellent"
	} else if perfData.ROI < plan.ROIExpectation*0.5 {
		status = "poor"
	} else if perfData.ROI < plan.ROIExpectation*0.8 {
		status = "needs_improvement"
	}

	return types.EvaluationResult{
		CampaignID:      plan.CampaignID,
		PerformanceData: *perfData,
		ROIExpected:     plan.ROIExpectation,
		ROIAchieved:     perfData.ROI,
		AchievementRate: perfData.ROI / plan.ROIExpectation * 100,
		Analysis: fmt.Sprintf("本次投放共展示%.0f次，点击%.0f次，转化%.0f笔带来收益%.2f元",
			float64(perfData.Impressions), float64(perfData.Clicks),
			float64(perfData.Conversions), perfData.Revenue),
		LessonsLearned: []string{
			"定向策略有效，18-35岁女性用户转化率高",
			"CPC控制在合理范围内",
			"建议增加晚间时段的投放",
		},
		Recommendations: []string{
			"优化创意素材，提高点击率",
			"适当提升出价获取更多曝光",
			"测试不同定向组合",
		},
		Status: status,
	}
}

// 沉淀经验 更新记忆库
func (c *AgentC) 沉淀经验(ctx context.Context, plan *types.CampaignPlan, result *types.EvaluationResult) {
	// 构建经验教训
	lesson := fmt.Sprintf("[%s] 产品:%s, 平台:%s, 实际ROI:%.2f, 预期ROI:%.2f, 达成率:%.1f%%, 状态:%s",
		time.Now().Format("2006-01-02"),
		plan.Product, "抖音",
		result.ROIAchieved, result.ROIExpected,
		result.AchievementRate, result.Status)

	// 保存到记忆
	c.WriteMemory(ctx, lesson, memory.MemoryMeta{
		Type:      memory.MemoryTypeLongTerm,
		Source:    "evaluation_lesson",
		Tags:      []string{plan.Product, "经验教训", result.Status},
		Importance: 8,
	})

	// 保存完整评估结果
	evalJSON, _ := json.Marshal(result)
	c.WriteMemory(ctx, fmt.Sprintf("评估结果: %s", string(evalJSON)), memory.MemoryMeta{
		Type:      memory.MemoryTypeLongTerm,
		Source:    "evaluation_result",
		Tags:      []string{plan.Product, "评估结果"},
		Importance: 9,
	})
}

// formatHistoricalResults 格式化历史评估结果
func (c *AgentC) formatHistoricalResults(results []*memory.SearchResult) string {
	if len(results) == 0 {
		return "暂无历史评估记录"
	}
	var result string
	for _, r := range results {
		result += fmt.Sprintf("- %s\n", r.Snippet)
	}
	return result
}

// GetPerformanceData 获取投放数据（模拟）
func (c *AgentC) GetPerformanceData(campaignIDs []string) *types.PerformanceData {
	// 模拟数据
	return &types.PerformanceData{
		CampaignID:  campaignIDs[0],
		Impressions: 125000,
		Clicks:     3750,
		CTR:        3.0,
		Spend:      5625.0,
		CPC:        1.5,
		Conversions: 187,
		CPA:        30.08,
		Revenue:    14025.0,
		ROI:        2.49,
		Timestamp:  time.Now(),
	}
}

// GetSessionStore 获取 Session Store
func (c *AgentC) GetSessionStore() session.SessionStore {
	return c.sessionStore
}
