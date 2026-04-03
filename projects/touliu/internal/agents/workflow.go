/*
 * Workflow Context - 工作流上下文
 */

package agents

import (
	"github.com/leejansq/clawgo/projects/touliu/internal/types"
)

// WorkflowContext 工作流上下文 (Graph 状态)
type WorkflowContext struct {
	State        *types.WorkflowState    // 工作流状态
	Product      string                  // 产品
	Platform     string                  // 平台
	TargetMarket string                  // 目标市场
	Step         int                     // 当前步骤
	Plan         *types.CampaignPlan     // 投放方案
	Execute      *types.ExecutionResult  // 执行结果
	Eval         *types.EvaluationResult // 评估结果
	Confirmed    bool                    // 是否已确认
	NeedsConfirm bool                    // 是否需要人工确认
	Error        string                  // 错误信息
}
