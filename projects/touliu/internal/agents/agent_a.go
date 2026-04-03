/*
 * Agent-A: 市场调研与策略师
 * 使用 LLM + Session + Memory + Skills
 */

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/leejansq/clawgo/internal/memory"
	"github.com/leejansq/clawgo/internal/session"
	"github.com/leejansq/clawgo/projects/touliu/internal/types"
)

// AgentA 市场调研与策略师
type AgentA struct {
	*AdAgent
}

// NewAgentA 创建市场调研与策略师
func NewAgentA(ctx context.Context, cfg *AdAgentConfig) (*AgentA, error) {
	cfg.Name = "Agent-A-策略师"
	cfg.Description = "市场调研与策略师：分析市场数据、竞品信息、历史投放记录，输出结构化投流方案"
	cfg.SystemPrompt = `你是电商投放策略专家，负责市场调研和制定投放方案。

你的职责：
1. 分析目标市场数据和趋势
2. 研究竞争对手的投放策略
3. 分析历史投放记录和效果
4. 结合知识库中的经验教训
5. 输出结构化的投流方案

输出格式必须是以下JSON结构（必须严格遵守字段名和类型）：

{
  "campaignId": "计划ID，自动生成",
  "product": "产品名称",
  "marketAnalysis": "市场分析报告（字符串）",
  "competitorInfo": "竞品分析（字符串）",
  "targeting": {
    "ageRange": [18, 35],        // 年龄范围，两个整数
    "gender": "all",             // male/female/all
    "locations": ["北京","上海"], // 地域数组
    "interests": ["电商","购物"], // 兴趣标签数组
    "deviceTypes": ["mobile"]    // 设备类型数组
  },
  "bid": {
    "bidType": "CPC",            // CPC/CPM/CPA
    "bidAmount": 1.5,            // 出价金额
    "dailyBudget": 1000,        // 日预算
    "totalBudget": 30000         // 总预算
  },
  "creativeBrief": "创意简报（字符串）",
  "roiExpectation": 2.5,       // 预期ROI（数字）
  "riskAssessment": "风险评估（字符串）",
  "recommendations": ["建议1", "建议2"]  // 建议数组
}

注意：
- targeting.ageRange 必须是 [minAge, maxAge] 格式的两个整数
- bid.bidType 必须是 CPC/CPM/CPA 之一
- 所有数值字段必须是数字类型，不能是字符串
- recommendations 必须是字符串数组`

	base, err := NewAdAgent(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &AgentA{AdAgent: base}, nil
}

// SystemPrompt 返回系统提示
func (a *AgentA) SystemPrompt() string {
	return a.AdAgent.systemPrompt
}

// AnalyzeAndPlan 市场分析并制定投放方案
func (a *AgentA) AnalyzeAndPlan(ctx context.Context, product, platform, targetMarket string) (*types.CampaignPlan, error) {
	// 1. 加载专业知识库
	knowledge, _ := a.LoadKnowledgeFiles()

	// 2. 从记忆获取相关经验
	lessons, _ := a.SearchMemory(ctx, fmt.Sprintf("%s %s 投放经验", product, platform), 5)
	historicalPlans, _ := a.SearchMemory(ctx, fmt.Sprintf("%s %s 投放方案", product, platform), 3)

	// 3. 构建分析任务
	task := fmt.Sprintf(`
请为以下产品制定投放方案：

产品信息：
- 产品名称：%s
- 投放平台：%s
- 目标市场：%s

专业知识库：
%s

相关经验教训（来自历史记忆）：
%s

历史投放方案：
%s

请分析以上信息，输出一个完整的投放方案JSON。
`, product, platform, targetMarket,
		knowledge,
		a.formatMemoryResults(lessons),
		a.formatMemoryResults(historicalPlans))

	// 3. 调用 LLM（buildMessages 会自动包含 session 历史）
	result, err := a.Chat(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("failed to execute analysis: %w", err)
	}

	// 4. 记录到 session
	a.AppendSession("assistant", result)

	// 5. 解析结果
	plan, err := a.parsePlan(result, product)
	if err != nil {
		return nil, fmt.Errorf("failed to parse plan: %w", err)
	}

	return plan, nil
}

// parsePlan 解析 JSON 方案
func (a *AgentA) parsePlan(jsonStr, product string) (*types.CampaignPlan, error) {
	//fmt.Printf("📝 [ParsePlan] 输入 JSON 字符串 (长度: %d): %s...\n", len(jsonStr), jsonStr)
	trimmed := strings.TrimSpace(jsonStr)
	if trimmed == "" {
		// LLM 返回空，直接用默认方案
		return a.defaultPlan(product), nil
	}

	// 尝试直接解析
	var plan types.CampaignPlan
	if err := json.Unmarshal([]byte(trimmed), &plan); err == nil {
		plan.CampaignID = generateID("plan")
		return &plan, nil
	}

	// 尝试提取 JSON 块
	return a.extractPlanFromText(trimmed, product)
}

// defaultPlan 创建默认投放方案
func (a *AgentA) defaultPlan(product string) *types.CampaignPlan {
	return &types.CampaignPlan{
		CampaignID:     generateID("plan"),
		Product:        product,
		MarketAnalysis: "基于市场数据生成的分析",
		Targeting: types.TargetingConfig{
			AgeRange:    [2]int{18, 35},
			Gender:      "all",
			Locations:   []string{"北京", "上海", "广州", "深圳"},
			Interests:   []string{"电商", "购物"},
			DeviceTypes: []string{"mobile"},
		},
		Bid: types.BidConfig{
			BidType:     "CPC",
			BidAmount:   1.5,
			DailyBudget: 1000,
			TotalBudget: 30000,
		},
		CreativeBrief:   "突出产品卖点，吸引目标用户",
		ROIExpectation:  2.5,
		RiskAssessment:  "市场波动风险，竞品跟进风险",
		Recommendations: []string{"持续优化创意", "关注数据变化及时调整出价"},
	}
}

// extractPlanFromText 从文本中提取 JSON 方案
func (a *AgentA) extractPlanFromText(text, product string) (*types.CampaignPlan, error) {
	fmt.Printf("📝 [extractPlanFromText] 原始输入长度: %d\n", len(text))

	// 优先处理 markdown 代码块格式 ```json ... ```
	if strings.Contains(text, "```json") {
		start := strings.Index(text, "```json")
		content := text[start+6:] // 跳过 ```json
		end := strings.Index(content, "```")
		if end != -1 {
			jsonStr := strings.TrimSpace(content[:end])
			// 确保从真正的 { 开始
			if idx := strings.Index(jsonStr, "{"); idx > 0 {
				jsonStr = jsonStr[idx:]
			}
			fmt.Printf("📝 [extractPlanFromText] 从 ```json 提取: %s\n", jsonStr[:min(100, len(jsonStr))])
			return a.parseJSON([]byte(jsonStr), product)
		}
	}

	// 处理普通 ``` ... ```
	if strings.Contains(text, "```") {
		start := strings.Index(text, "```")
		content := text[start+3:]
		end := strings.Index(content, "```")
		if end != -1 {
			jsonStr := strings.TrimSpace(content[:end])
			// 确保从真正的 { 开始
			if idx := strings.Index(jsonStr, "{"); idx > 0 {
				jsonStr = jsonStr[idx:]
			}
			if strings.Contains(jsonStr, "{") {
				fmt.Printf("📝 [extractPlanFromText] 从 ``` 提取: %s\n", jsonStr[:min(100, len(jsonStr))])
				return a.parseJSON([]byte(jsonStr), product)
			}
		}
	}

	// 查找第一个 { 到最后一个 } 的 JSON 块
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")

	if start == -1 || end == -1 || end <= start {
		fmt.Printf("❌ [extractPlanFromText] 无法找到 JSON 块\n")
		return a.defaultPlan(product), nil
	}

	jsonStr := text[start : end+1]
	fmt.Printf("📝 [extractPlanFromText] 查找 {..}: %s\n", jsonStr[:min(100, len(jsonStr))])
	return a.parseJSON([]byte(jsonStr), product)
}

// parseJSON 解析 JSON 字节数组
func (a *AgentA) parseJSON(jsonBytes []byte, product string) (*types.CampaignPlan, error) {
	// 清理 JSON 字符串：去除多余空白、换行等
	jsonStr := strings.TrimSpace(string(jsonBytes))

	// 移除 BOM 和其他隐藏字符
	jsonStr = strings.TrimPrefix(jsonStr, "\xef\xbb\xbf")
	jsonStr = strings.TrimPrefix(jsonStr, "\ufeff")

	// 移除 markdown 代码块标记
	jsonStr = strings.TrimPrefix(jsonStr, "json")
	jsonStr = strings.TrimSpace(jsonStr)

	var plan types.CampaignPlan
	if err := json.Unmarshal([]byte(jsonStr), &plan); err != nil {
		fmt.Printf("❌ [parseJSON] 解析失败: %v\n", err)
		fmt.Printf("   清理后的 JSON: %s\n", jsonStr[:min(200, len(jsonStr))])
		return a.defaultPlan(product), nil
	}

	plan.CampaignID = generateID("plan")
	plan.Product = product
	return &plan, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (a *AgentA) formatMemoryResults(results []*memory.SearchResult) string {
	if len(results) == 0 {
		return "暂无相关记录"
	}
	var result string
	for i, r := range results {
		result += fmt.Sprintf("%d. [%s] %s\n", i+1, r.Source, r.Snippet)
	}
	return result
}

// GetSessionStore 获取 Session Store
func (a *AgentA) GetSessionStore() session.SessionStore {
	return a.sessionStore
}

// RevisePlan 根据人工反馈修订投放方案
func (a *AgentA) RevisePlan(ctx context.Context, plan *types.CampaignPlan, feedback string) (*types.CampaignPlan, error) {
	// 0. 调试：打印反馈信息
	fmt.Printf("\n📝 [RevisePlan] 收到人工反馈: %s\n", feedback)

	// 1. 将当前方案和反馈构建为修订任务
	task := fmt.Sprintf(`
请根据以下人工反馈修订投放方案：

当前方案：
%s

人工反馈意见：
%s

请分析反馈内容，对方案进行针对性修订，输出修订后的完整投放方案JSON。

JSON格式要求（必须严格遵守）：
{
  "campaignId": "计划ID，自动生成",
  "product": "产品名称",
  "marketAnalysis": "市场分析报告（字符串）",
  "competitorInfo": "竞品分析（字符串）",
  "targeting": {
    "ageRange": [18, 35],        // 年龄范围，两个整数
    "gender": "all",             // male/female/all
    "locations": ["北京","上海"], // 地域数组
    "interests": ["电商","购物"], // 兴趣标签数组
    "deviceTypes": ["mobile"]    // 设备类型数组
  },
  "bid": {
    "bidType": "CPC",
    "bidAmount": 1.5,
    "dailyBudget": 1000,
    "totalBudget": 30000
  },
  "creativeBrief": "创意简报（字符串）",
  "roiExpectation": 2.5,
  "riskAssessment": "风险评估（字符串）",
  "recommendations": ["建议1", "建议2"]
}

注意：
- targeting.ageRange 必须是 [minAge, maxAge] 格式的两个整数
- bid.bidType 必须是 CPC/CPM/CPA 之一
- 所有数值字段必须是数字类型
- recommendations 必须是字符串数组
`, a.formatPlan(plan), feedback)

	fmt.Printf("📝 [RevisePlan] 修订任务已构建，准备调用 LLM...\n")

	// 2. 调用 LLM（buildMessages 会自动包含 session 历史）
	fmt.Printf("📝 [RevisePlan] 调用 Chat 方法...\n")
	result, err := a.Chat(ctx, task)
	if err != nil {
		fmt.Printf("❌ [RevisePlan] Chat 调用失败: %v\n", err)
		return nil, fmt.Errorf("failed to revise plan: %w", err)
	}

	fmt.Printf("📝 [RevisePlan] Chat 返回结果 (长度: %d): %.200s...\n", len(result), result)

	// 3. 记录到 session
	a.AppendSession("assistant", result)

	// 4. 解析结果
	newPlan, err := a.parsePlan(result, plan.Product)
	if err != nil {
		fmt.Printf("❌ [RevisePlan] 解析方案失败: %v\n", err)
		return nil, fmt.Errorf("failed to parse revised plan: %w", err)
	}

	fmt.Printf("✅ [RevisePlan] 方案修订成功: ROI=%.2f, 预算=%.2f\n", newPlan.ROIExpectation, newPlan.Bid.DailyBudget)

	return newPlan, nil
}

// formatPlan 将方案格式化为字符串
func (a *AgentA) formatPlan(plan *types.CampaignPlan) string {
	data, _ := json.Marshal(plan)
	return string(data)
}
