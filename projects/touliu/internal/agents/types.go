/*
 * E-Commerce Advertising Multi-Agent System
 * Re-export types from internal/types
 */

package agents

import "github.com/leejansq/clawgo/projects/touliu/internal/types"

// Re-export types for convenience
type CampaignStatus = types.CampaignStatus
type TargetingConfig = types.TargetingConfig
type BidConfig = types.BidConfig
type Campaign = types.Campaign
type CampaignPlan = types.CampaignPlan
type ExecutionResult = types.ExecutionResult
type PerformanceData = types.PerformanceData
type EvaluationResult = types.EvaluationResult
type WorkflowState = types.WorkflowState

// Constants re-export
const (
	StatusPending    = types.StatusPending
	StatusRunning    = types.StatusRunning
	StatusPaused    = types.StatusPaused
	StatusCompleted = types.StatusCompleted
	StatusFailed    = types.StatusFailed
)
