/*
 * E-Commerce Advertising Multi-Agent System
 * Type definitions for the advertising workflow
 */

package types

import "time"

// CampaignStatus 投放状态
type CampaignStatus string

const (
	StatusPending    CampaignStatus = "pending"
	StatusRunning    CampaignStatus = "running"
	StatusPaused     CampaignStatus = "paused"
	StatusCompleted  CampaignStatus = "completed"
	StatusFailed     CampaignStatus = "failed"
)

// TargetingConfig 定向配置
type TargetingConfig struct {
	AgeRange    [2]int    `json:"ageRange"`    // 年龄范围，如 [18, 35]
	Gender      string    `json:"gender"`      // male/female/all
	Locations   []string  `json:"locations"`   // 投放地域
	Interests   []string  `json:"interests"`   // 兴趣标签
	DeviceTypes []string  `json:"deviceTypes"` // 设备类型
}

// BidConfig 出价配置
type BidConfig struct {
	BidType     string  `json:"bidType"`     // CPC/CPM/CPA
	BidAmount   float64 `json:"bidAmount"`   // 出价金额
	DailyBudget float64 `json:"dailyBudget"` // 日预算
	TotalBudget float64 `json:"totalBudget"` // 总预算
}

// Campaign 投放计划
type Campaign struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Product     string         `json:"product"`
	Platform    string         `json:"platform"` // 平台：douyin/weibo/toutiao
	Status      CampaignStatus `json:"status"`
	Targeting   TargetingConfig `json:"targeting"`
	Bid         BidConfig      `json:"bid"`
	CreativeURL string         `json:"creativeUrl"` // 创意素材URL
	StartDate   time.Time      `json:"startDate"`
	EndDate     time.Time      `json:"endDate"`
	CreatedAt   time.Time      `json:"createdAt"`
}

// CampaignPlan 投放方案 (Agent-A 输出)
type CampaignPlan struct {
	CampaignID      string            `json:"campaignId"`
	Product         string            `json:"product"`
	MarketAnalysis  string            `json:"marketAnalysis"`   // 市场分析
	CompetitorInfo  string            `json:"competitorInfo"`   // 竞品信息
	Targeting       TargetingConfig   `json:"targeting"`
	Bid            BidConfig         `json:"bid"`
	CreativeBrief  string            `json:"creativeBrief"`   // 创意简报
	ROIExpectation float64           `json:"roiExpectation"`  // 预期ROI
	RiskAssessment string            `json:"riskAssessment"`  // 风险评估
	Recommendations []string         `json:"recommendations"` // 建议
}

// ExecutionResult 投放执行结果 (Agent-B 输出)
type ExecutionResult struct {
	CampaignID  string    `json:"campaignId"`
	PlanID      string    `json:"planId"`
	Status     string    `json:"status"`           // success/failed/partial
	CampaignIDs []string  `json:"campaignIds"`      // 创建的计划ID列表
	ErrorMsg    string    `json:"errorMsg,omitempty"`
	ExecutedAt  time.Time `json:"executedAt"`
	Details     string    `json:"details"` // 执行详情
}

// PerformanceData 投放数据 (Agent-C 监控)
type PerformanceData struct {
	CampaignID  string    `json:"campaignId"`
	Impressions int64     `json:"impressions"` // 展示量
	Clicks     int64     `json:"clicks"`      // 点击量
	CTR        float64   `json:"ctr"`         // 点击率
	Spend      float64   `json:"spend"`       // 花费
	CPC        float64   `json:"cpc"`         // 点击成本
	Conversions int64    `json:"conversions"` // 转化数
	CPA        float64   `json:"cpa"`         // 转化成本
	Revenue    float64   `json:"revenue"`     // 带来的收益
	ROI        float64   `json:"roi"`         // 投资回报率
	Timestamp  time.Time `json:"timestamp"`
}

// EvaluationResult 评估结果 (Agent-C 输出)
type EvaluationResult struct {
	CampaignID      string           `json:"campaignId"`
	PerformanceData PerformanceData  `json:"performanceData"`
	ROIAchieved     float64          `json:"roiAchieved"`
	ROIExpected     float64          `json:"roiExpected"`
	AchievementRate float64          `json:"achievementRate"` // ROI达成率
	Analysis        string           `json:"analysis"`       // 效果分析
	LessonsLearned  []string         `json:"lessonsLearned"` // 经验教训
	Recommendations []string         `json:"recommendations"` // 优化建议
	Status          string           `json:"status"`         // excellent/good/needs_improvement/poor
}

// WorkflowState 工作流状态
type WorkflowState struct {
	CurrentStep   int                `json:"currentStep"`    // 1=策略, 2=执行, 3=评估
	Plan          *CampaignPlan      `json:"plan,omitempty"`
	Execution     *ExecutionResult    `json:"execution,omitempty"`
	Evaluation    *EvaluationResult   `json:"evaluation,omitempty"`
	OverallStatus string              `json:"overallStatus"`
	CreatedAt     time.Time          `json:"createdAt"`
	UpdatedAt     time.Time          `json:"updatedAt"`
}
