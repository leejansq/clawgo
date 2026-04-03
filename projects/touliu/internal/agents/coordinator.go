/*
 * Workflow Coordinator - 工作流协调器
 * 使用 eino Graph 进行 Agent 执行编排
 * 支持 Human-in-Loop、并行执行、条件路由等
 */

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/leejansq/clawgo/internal/skill"
	"github.com/leejansq/clawgo/projects/touliu/internal/terminal"
	"github.com/leejansq/clawgo/projects/touliu/internal/types"
)

// Coordinator 工作流协调器
type Coordinator struct {
	graph         compose.Runnable[*WorkflowContext, *WorkflowContext]
	agentA        *AgentA
	agentB        *AgentB
	agentC        *AgentC
	state         *types.WorkflowState
	workspace     string
	knowledgePath string
}

// CoordinatorConfig 协调器配置
type CoordinatorConfig struct {
	Model            model.ChatModel
	ToolCallingModel model.ToolCallingChatModel
	Workspace        string
	KnowledgePath    string // 专业知识库路径
	SkillDirs        []string
}

// NewCoordinator 创建协调器
func NewCoordinator(cfg *CoordinatorConfig) (*Coordinator, error) {
	ctx := context.Background()

	// 构建技能源
	var sources []skill.SkillSource
	for i, dir := range cfg.SkillDirs {
		sources = append(sources, skill.SkillSource{
			Path:     dir,
			Priority: 100 - i,
			Label:    fmt.Sprintf("skill-source-%d", i),
		})
	}

	// Agent 配置
	agentCfg := &AdAgentConfig{
		Model:             cfg.Model,
		ToolCallingModel:   cfg.ToolCallingModel,
		SessionCWD:        cfg.Workspace,
		MemoryBaseDir:     cfg.Workspace + "/memory",
		KnowledgeBaseDir:  cfg.KnowledgePath,
		SkillSources:      sources,
	}

	// 创建各个 Agent
	agentA, _ := NewAgentA(ctx, agentCfg)
	agentCfgB := *agentCfg
	agentB, _ := NewAgentB(ctx, &agentCfgB)
	agentCfgC := *agentCfg
	agentC, _ := NewAgentC(ctx, &agentCfgC)

	c := &Coordinator{
		agentA:        agentA,
		agentB:        agentB,
		agentC:        agentC,
		workspace:     cfg.Workspace,
		knowledgePath: cfg.KnowledgePath,
		state: &types.WorkflowState{
			CurrentStep:   0,
			OverallStatus: "initialized",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		},
	}

	return c, nil
}

// BuildGraph 构建 Graph (供外部调用以支持复杂编排)
func (c *Coordinator) BuildGraph(ctx context.Context) (compose.Runnable[*WorkflowContext, *WorkflowContext], error) {
	// 创建 Graph，使用 workflowState 作为上下文
	g := compose.NewGraph[*WorkflowContext, *WorkflowContext](
		compose.WithGenLocalState(func(ctx context.Context) *WorkflowContext {
			return &WorkflowContext{
				State:        c.state,
				Product:     "",
				Platform:    "",
				TargetMarket: "",
				Step:        0,
				Plan:        nil,
				Execute:     nil,
				Eval:        nil,
				Confirmed:   false,
				Error:       "",
			}
		}),
	)

	// ========== 1. Agent-A: 市场调研与策略节点 ==========
	agentALambda := compose.InvokableLambda(
		func(ctx context.Context, input *WorkflowContext) (output *WorkflowContext, err error) {
			if input.Product == "" {
				return input, nil
			}
			plan, err := c.agentA.AnalyzeAndPlan(ctx, input.Product, input.Platform, input.TargetMarket)
			if err != nil {
				input.State.OverallStatus = "failed"
				input.Error = err.Error()
				return input, err
			}
			input.Plan = plan
			input.Step = 1
			input.State.Plan = plan
			input.State.CurrentStep = 1
			input.State.UpdatedAt = time.Now()
			return input, nil
		},
		compose.WithLambdaType("AgentA"),
	)

	// ========== 2. Human-Confirm: 人工确认节点 ==========
	confirmLambda := compose.InvokableLambda(
		func(ctx context.Context, input *WorkflowContext) (output *WorkflowContext, err error) {
			if input.Plan == nil {
				return input, nil
			}
			// 标记需要确认
			input.NeedsConfirm = true
			return input, nil
		},
		compose.WithLambdaType("HumanConfirm"),
	)

	// ========== 3. Agent-B: 投放执行节点 ==========
	agentBLambda := compose.InvokableLambda(
		func(ctx context.Context, input *WorkflowContext) (output *WorkflowContext, err error) {
			if !input.Confirmed || input.Plan == nil {
				return input, nil
			}
			exec := c.agentB.SimulateExecution(input.Plan)
			input.Execute = exec
			input.Step = 2
			input.State.Execution = exec
			input.State.CurrentStep = 2
			input.State.UpdatedAt = time.Now()
			return input, nil
		},
		compose.WithLambdaType("AgentB"),
	)

	// ========== 4. Agent-C: 效果评估节点 ==========
	agentCLambda := compose.InvokableLambda(
		func(ctx context.Context, input *WorkflowContext) (output *WorkflowContext, err error) {
			if input.Execute == nil || input.Plan == nil {
				return input, nil
			}
			perfData := c.agentC.GetPerformanceData(input.Execute.CampaignIDs)
			eval, err := c.agentC.Evaluate(ctx, input.Plan, input.Execute, perfData)
			if err != nil {
				input.State.OverallStatus = "failed"
				input.Error = err.Error()
				return input, err
			}
			input.Eval = eval
			input.Step = 3
			input.State.Evaluation = eval
			input.State.CurrentStep = 3
			input.State.OverallStatus = "completed"
			input.State.UpdatedAt = time.Now()
			return input, nil
		},
		compose.WithLambdaType("AgentC"),
	)

	// ========== 添加节点到 Graph ==========
	g.AddLambdaNode("agent_a", agentALambda)
	g.AddLambdaNode("human_confirm", confirmLambda)
	g.AddLambdaNode("agent_b", agentBLambda)
	g.AddLambdaNode("agent_c", agentCLambda)

	// ========== 设置边 (顺序执行) ==========
	g.AddEdge(compose.START, "agent_a")
	g.AddEdge("agent_a", "human_confirm")
	g.AddEdge("human_confirm", "agent_b")
	g.AddEdge("agent_b", "agent_c")
	g.AddEdge("agent_c", compose.END)

	// ========== 添加条件分支: human_confirm 根据确认结果决定是否继续 ==========
	// 注意: 人工确认已在 RunWithGraph 中通过 stdin 完成，Confirmed 字段在调用图之前已设置
	branch := compose.NewGraphBranch(
		func(ctx context.Context, input *WorkflowContext) (string, error) {
			if input.Confirmed {
				return "agent_b", nil
			}
			return compose.END, nil
		},
		map[string]bool{"agent_b": true, compose.END: true},
	)
	g.AddBranch("human_confirm", branch)

	// 编译 Graph
	graph, err := g.Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to compile graph: %w", err)
	}

	c.graph = graph
	return graph, nil
}

// RunWithGraph 使用 Graph 执行工作流
func (c *Coordinator) RunWithGraph(ctx context.Context, product, platform, targetMarket string) (*types.WorkflowState, error) {
	renderer := terminal.NewRenderer()

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println(renderer.Theme.EmphasisStyle("  🤖 电商投流多 Agent 系统"))
	fmt.Println("========================================")
	fmt.Printf("产品: %s | 平台: %s | 市场: %s\n", product, platform, targetMarket)
	fmt.Println("========================================")
	fmt.Println()

	// 构建 Graph (如果尚未构建)
	if c.graph == nil {
		_, err := c.BuildGraph(ctx)
		if err != nil {
			return nil, err
		}
	}

	// 初始化上下文
	wfCtx := &WorkflowContext{
		State:        c.state,
		Product:      product,
		Platform:     platform,
		TargetMarket: targetMarket,
		Step:         0,
		Confirmed:    false,
	}

	// ========== Step 1: Agent-A 执行 ==========
	fmt.Println("----------------------------------------")
	fmt.Println(renderer.Theme.HeadingStyle("  [Step 1] Agent-A: 市场调研与策略分析"))
	fmt.Println("----------------------------------------")
	c.state.CurrentStep = 1

	plan, err := c.agentA.AnalyzeAndPlan(ctx, product, platform, targetMarket)
	if err != nil {
		c.state.OverallStatus = "failed"
		return c.state, fmt.Errorf("Agent-A failed: %w", err)
	}
	c.state.Plan = plan
	wfCtx.Plan = plan
	fmt.Printf("  %s 方案已生成: %s\n", renderer.Theme.SuccessStyle("✔"), plan.CampaignID)
	fmt.Printf("  预期ROI: %.2f | 定向: %d-%d岁 | 预算: %.2f元/天\n",
		plan.ROIExpectation, plan.Targeting.AgeRange[0], plan.Targeting.AgeRange[1], plan.Bid.DailyBudget)
	fmt.Println()

	// ========== Human-in-Loop: 多轮修订循环 ==========
	plan, confirmed, err := c.humanAgentAReviewLoop(ctx, plan, platform)
	if err != nil {
		c.state.OverallStatus = "failed"
		return c.state, fmt.Errorf("human review failed: %w", err)
	}
	if !confirmed {
		// 用户取消
		return c.state, nil
	}
	c.state.Plan = plan
	wfCtx.Plan = plan
	wfCtx.Confirmed = true

	// ========== Step 2: Agent-B 执行 ==========
	fmt.Println("----------------------------------------")
	fmt.Println(renderer.Theme.HeadingStyle("  [Step 2] Agent-B: 投放执行"))
	fmt.Println("----------------------------------------")
	c.state.CurrentStep = 2

	execution := c.agentB.SimulateExecution(plan)
	c.state.Execution = execution
	wfCtx.Execute = execution
	fmt.Printf("  %s 执行完成: %s\n", renderer.Theme.SuccessStyle("✔"), execution.Status)
	fmt.Printf("  创建计划: %v\n", execution.CampaignIDs)
	fmt.Println()

	// ========== Step 3: Agent-C 执行 ==========
	fmt.Println("----------------------------------------")
	fmt.Println(renderer.Theme.HeadingStyle("  [Step 3] Agent-C: 效果评估"))
	fmt.Println("----------------------------------------")
	c.state.CurrentStep = 3

	perfData := c.agentC.GetPerformanceData(execution.CampaignIDs)
	evaluation, err := c.agentC.Evaluate(ctx, plan, execution, perfData)
	if err != nil {
		c.state.OverallStatus = "failed"
		return c.state, fmt.Errorf("Agent-C failed: %w", err)
	}
	c.state.Evaluation = evaluation
	c.state.OverallStatus = "completed"
	c.state.UpdatedAt = time.Now()

	fmt.Printf("  %s 评估完成: %s\n", renderer.Theme.SuccessStyle("✔"), evaluation.Status)
	fmt.Printf("  实际ROI: %.2f (预期: %.2f) | 达成率: %.1f%%\n",
		evaluation.ROIAchieved, evaluation.ROIExpected, evaluation.AchievementRate)

	if len(evaluation.LessonsLearned) > 0 {
		fmt.Println()
		fmt.Println(renderer.Theme.EmphasisStyle("  📝 经验教训:"))
		for i, lesson := range evaluation.LessonsLearned {
			fmt.Printf("    %d. %s\n", i+1, lesson)
		}
	}

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println(renderer.Theme.SuccessStyle("  ✔ 工作流执行完成!"))
	fmt.Println("========================================")
	fmt.Println()

	return c.state, nil
}

// Run 执行工作流 (简版顺序执行，用于演示)
func (c *Coordinator) Run(ctx context.Context, product, platform, targetMarket string) (*types.WorkflowState, error) {
	return c.RunWithGraph(ctx, product, platform, targetMarket)
}

// GetGraph 获取构建好的 Graph
func (c *Coordinator) GetGraph() compose.Runnable[*WorkflowContext, *WorkflowContext] {
	return c.graph
}

// GetState 获取工作流状态
func (c *Coordinator) GetState() *types.WorkflowState {
	return c.state
}

// GetAgentA 获取 Agent-A
func (c *Coordinator) GetAgentA() *AgentA {
	return c.agentA
}

// GetAgentB 获取 Agent-B
func (c *Coordinator) GetAgentB() *AgentB {
	return c.agentB
}

// GetAgentC 获取 Agent-C
func (c *Coordinator) GetAgentC() *AgentC {
	return c.agentC
}

// Stop 停止所有 Agent
func (c *Coordinator) Stop() {
	if c.agentA != nil {
		c.agentA.Close()
	}
	if c.agentB != nil {
		c.agentB.Close()
	}
	if c.agentC != nil {
		c.agentC.Close()
	}
}

// WorkflowStateToJSON 将工作流状态转换为 JSON
func WorkflowStateToJSON(state *types.WorkflowState) string {
	data, _ := json.MarshalIndent(state, "", "  ")
	return string(data)
}

// =============================================================================
// Human-in-Loop 多轮修订循环
// =============================================================================

// humanAgentAReviewLoop 人工与 Agent-A 的多轮修订循环
// 返回最终确认的方案、是否确认成功、错误
func (c *Coordinator) humanAgentAReviewLoop(ctx context.Context, plan *types.CampaignPlan, platform string) (*types.CampaignPlan, bool, error) {
	editor := terminal.NewLineEditor()
	renderer := terminal.NewRenderer()
	feedback := ""
	reviseCount := 0

	for {
		// 显示方案详情
		c.displayPlanWithRenderer(plan, platform, renderer)

		// 显示确认提示
		editor.DisplayPrompt(reviseCount)

		// 读取用户输入
		decision, extra, err := editor.ReadDecisionWithSlash()
		if err != nil {
			return plan, false, err
		}

		switch decision {
		case terminal.Approve:
			// 确认方案
			fmt.Println()
			fmt.Println(renderer.Theme.SuccessStyle("  ✔ 方案已确认，开始执行..."))
			fmt.Println()
			return plan, true, nil

		case terminal.Revise:
			// 检查特殊指令
			switch extra {
			case "__SHOW_JSON__":
				// 显示 JSON
				fmt.Println()
				fmt.Println(renderer.RenderJSON(plan))
				fmt.Println()
				continue

			case "__SHOW_HELP__":
				// 显示帮助
				fmt.Println()
				fmt.Println(renderer.RenderHelp())
				fmt.Println()
				continue

			case "":
				// 需要读取反馈
				feedback, err = editor.ReadFeedback("请输入修改意见")
				if err != nil {
					return plan, false, err
				}
				if feedback == "" {
					fmt.Println(renderer.Theme.WarningStyle("  ⚠ 未输入修改意见，请重试"))
					continue
				}

			default:
				// extra 包含反馈内容
				feedback = extra
			}

			// 修订方案
			fmt.Println()
			spinner := terminal.NewSpinner()
			spinner.Tick("正在修订方案...")
			fmt.Println()

			newPlan, err := c.agentA.RevisePlan(ctx, plan, feedback)
			if err != nil {
				spinner.Stop("方案修订失败", false)
				fmt.Printf("  %s\n", renderer.Theme.ErrorStyle(fmt.Sprintf("⚠ 修订失败: %v，请重试", err)))
				continue
			}

			spinner.Stop("方案修订完成", true)
			plan = newPlan
			reviseCount++
			fmt.Println(renderer.Theme.SuccessStyle(fmt.Sprintf("  ✅ 方案已修订 (第 %d 轮)", reviseCount)))
			fmt.Println()
			continue

		case terminal.Reject:
			// 拒绝
			fmt.Println()
			fmt.Println(renderer.Theme.ErrorStyle("  ✘ 用户拒绝，执行取消"))
			c.state.OverallStatus = "cancelled"
			return nil, false, nil

		case terminal.Quit:
			// 退出
			fmt.Println()
			fmt.Println(renderer.Theme.WarningStyle("  ⚠ 用户退出"))
			c.state.OverallStatus = "cancelled"
			return nil, false, nil
		}
	}
}

// displayPlan 显示方案详情
func (c *Coordinator) displayPlan(plan *types.CampaignPlan, platform string) {
	renderer := terminal.NewRenderer()
	c.displayPlanWithRenderer(plan, platform, renderer)
}

// displayPlanWithRenderer 使用指定渲染器显示方案详情
func (c *Coordinator) displayPlanWithRenderer(plan *types.CampaignPlan, platform string, renderer *terminal.Renderer) {
	fmt.Println()
	fmt.Println(renderer.RenderPlan(plan, platform))
	fmt.Println()
}

// =============================================================================
// 以下是兼容 eino Graph 接口的 Agent Node 实现
// =============================================================================

// AgentNode 是一个兼容 eino Graph 接口的 Agent 封装
// 可用于 Chain、Parallel 等复杂编排
type AgentNode struct {
	agent *AdAgent
	name  string
}

// NewAgentNode 创建 Agent Node
func NewAgentNode(name string, agent *AdAgent) *AgentNode {
	return &AgentNode{
		agent: agent,
		name:  name,
	}
}

// Invoke 实现 node.Invoker 接口
func (n *AgentNode) Invoke(ctx context.Context, input string, opts ...tool.Option) (string, error) {
	result, err := n.agent.Chat(ctx, input)
	if err != nil {
		return "", err
	}
	return result, nil
}

// Info 返回工具信息
func (n *AgentNode) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: n.name,
		Desc: fmt.Sprintf("Agent Node: %s", n.name),
	}, nil
}

// CreateAgentTools 创建 Agent 工具列表
// 返回的 Tools 可以用于创建 ToolsNode
func CreateAgentTools(agent *AdAgent) []tool.BaseTool {
	return []tool.BaseTool{
		NewAgentToolAdapter(agent),
	}
}

// AgentToolAdapter Agent 工具适配器
type AgentToolAdapter struct {
	agent *AdAgent
	name  string
}

// NewAgentToolAdapter 创建 Agent 工具适配器
func NewAgentToolAdapter(agent *AdAgent) *AgentToolAdapter {
	return &AgentToolAdapter{
		agent: agent,
		name:  agent.name,
	}
}

// Info 返回工具信息
func (t *AgentToolAdapter) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "agent_" + t.agent.name,
		Desc: fmt.Sprintf("调用 %s 执行任务", t.agent.name),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"task": {
				Type:    schema.String,
				Desc:    "任务描述",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun 执行任务
func (t *AgentToolAdapter) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var params struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", err
	}
	return t.agent.Chat(ctx, params.Task)
}
