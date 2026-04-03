/*
 * Director Agent - 导演智能体
 * 负责协调编剧、研究员、评论家三个子智能体的工作
 */

package director

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/leejansq/clawgo/projects/video/internal/critic"
	"github.com/leejansq/clawgo/projects/video/internal/manager"
	"github.com/leejansq/clawgo/projects/video/internal/researcher"
	"github.com/leejansq/clawgo/projects/video/internal/scriptwriter"
	"github.com/leejansq/clawgo/projects/video/pkg/schema"
)

const (
	// 最大迭代次数
	maxIterations = 3
	// 默认视频时长
	defaultDuration = 120
)

// Director 导演智能体
type Director struct {
	mgr         *manager.Manager
	researcher  *researcher.Researcher
	scriptwriter *scriptwriter.Scriptwriter
	critic      *critic.Critic
}

// GenerationTask 生成任务
type GenerationTask struct {
	SessionKey string
	Request    *schema.VideoScriptRequest
	Status     string // pending, running, completed, failed
	Result     *schema.GenerationResult
	Error      error
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewDirector 创建导演智能体
func NewDirector(mgr *manager.Manager) *Director {
	return &Director{
		mgr:         mgr,
		researcher:  researcher.NewResearcher(mgr),
		scriptwriter: scriptwriter.NewScriptwriter(mgr),
		critic:      critic.NewCritic(mgr),
	}
}

// Generate 生成视频脚本
func (d *Director) Generate(ctx context.Context, req *schema.VideoScriptRequest) (*schema.GenerationResult, error) {
	// 设置默认值
	if req.Duration == 0 {
		req.Duration = defaultDuration
	}
	if req.Language == "" {
		req.Language = "中文"
	}

	// 第一步：研究员搜集资料
	researchInput := &schema.ResearcherInput{
		Theme:   req.Theme,
		Aspects: []string{"最新动态", "数据统计", "案例分析", "发展趋势"},
	}

	researchResult, err := d.researcher.Research(ctx, researchInput)
	if err != nil {
		return nil, fmt.Errorf("research failed: %w", err)
	}

	// 整理研究资料
	researchData := d.formatResearchData(researchResult)

	// 第二步：编剧生成初稿
	currentScript, err := d.scriptwriter.WriteScript(ctx, &schema.ScriptwriterInput{
		Theme:          req.Theme,
		TargetAudience: req.TargetAudience,
		Duration:       req.Duration,
		ResearchData:   researchData,
		Iteration:      1,
	})
	if err != nil {
		return nil, fmt.Errorf("script generation failed: %w", err)
	}

	// 第三步：评论家审查（可能需要迭代）
	iteration := 0
	var lastCriticResult *schema.CriticOutput

	for iteration < maxIterations {
		iteration++

		criticInput := &schema.CriticInput{
			Script: d.formatScriptForReview(currentScript),
			Theme:  req.Theme,
		}

		criticResult, err := d.critic.Review(ctx, criticInput)
		if err != nil {
			// 审查失败，继续尝试
			lastCriticResult = &schema.CriticOutput{
				Approved: false,
				Feedback: fmt.Sprintf("审查失败: %v", err),
			}
		} else {
			lastCriticResult = criticResult
		}

		// 如果通过或达到最大迭代次数，退出
		if lastCriticResult.Approved || iteration >= maxIterations {
			break
		}

		// 评论家反馈需要修改，编剧重新撰写
		currentScript, err = d.scriptwriter.WriteScript(ctx, &schema.ScriptwriterInput{
			Theme:          req.Theme,
			TargetAudience: req.TargetAudience,
			Duration:       req.Duration,
			ResearchData:   researchData,
			Iteration:      iteration + 1,
			PreviousScript: d.formatScriptForReview(currentScript),
			CriticFeedback: lastCriticResult.Feedback,
		})
		if err != nil {
			return nil, fmt.Errorf("script revision failed: %w", err)
		}
	}

	// 第四步：整合最终脚本
	finalScript := d.integrateScript(currentScript, researchResult, iteration)

	// 构建结果
	result := &schema.GenerationResult{
		SessionKey:  fmt.Sprintf("video:%d", time.Now().UnixNano()),
		Status:      "completed",
		FinalScript: finalScript,
		SubResults: []schema.SubResult{
			{
				AgentType: "researcher",
				Output:    researchData,
				Timestamp: time.Now(),
			},
			{
				AgentType: "scriptwriter",
				Output:    d.formatScriptForReview(currentScript),
				Timestamp: time.Now(),
			},
			{
				AgentType: "critic",
				Output:    lastCriticResult.Feedback,
				Timestamp: time.Now(),
			},
		},
	}

	return result, nil
}

// GenerateAsync 异步生成视频脚本
func (d *Director) GenerateAsync(ctx context.Context, req *schema.VideoScriptRequest) (string, error) {
	sessionKey := fmt.Sprintf("video:%d", time.Now().UnixNano())

	go func() {
		result, err := d.Generate(context.Background(), req)
		if err != nil {
			result = &schema.GenerationResult{
				SessionKey: sessionKey,
				Status:     "failed",
				Error:      err.Error(),
			}
		}
		// 存储结果（这里可以存储到内存或数据库）
		_ = result
	}()

	return sessionKey, nil
}

// formatResearchData 格式化研究资料
func (d *Director) formatResearchData(r *schema.ResearcherOutput) string {
	var buf string

	if r.Overview != "" {
		buf += fmt.Sprintf("【主题概述】\n%s\n\n", r.Overview)
	}

	if len(r.KeyData) > 0 {
		buf += "【关键数据】\n"
		for _, d := range r.KeyData {
			buf += fmt.Sprintf("- %s\n", d)
		}
		buf += "\n"
	}

	if len(r.CaseStudies) > 0 {
		buf += "【案例分析】\n"
		for _, c := range r.CaseStudies {
			buf += fmt.Sprintf("- %s: %s\n", c.Title, c.Desc)
		}
		buf += "\n"
	}

	if len(r.Trends) > 0 {
		buf += "【发展趋势】\n"
		for _, t := range r.Trends {
			buf += fmt.Sprintf("- %s\n", t)
		}
		buf += "\n"
	}

	if len(r.FunFacts) > 0 {
		buf += "【有趣的事实】\n"
		for _, f := range r.FunFacts {
			buf += fmt.Sprintf("- %s\n", f)
		}
		buf += "\n"
	}

	return buf
}

// formatScriptForReview 格式化分镜头脚本用于审查
func (d *Director) formatScriptForReview(s *schema.VideoScript) string {
	if s == nil {
		return ""
	}

	var buf string

	if s.Title != "" {
		buf += fmt.Sprintf("【标题】%s\n", s.Title)
	}
	if s.Introduction != "" {
		buf += fmt.Sprintf("【开场介绍】%s\n", s.Introduction)
	}

	if len(s.Scenes) > 0 {
		buf += "\n【分镜头脚本】\n"
		for _, scene := range s.Scenes {
			buf += fmt.Sprintf("\n--- 镜头 %d (%d秒) ---\n", scene.Index, scene.Duration)
			buf += fmt.Sprintf("景别: %s\n", scene.Description)
			buf += fmt.Sprintf("运镜: %s\n", scene.CameraMove)
			buf += fmt.Sprintf("画面: %s\n", scene.Visual)
			buf += fmt.Sprintf("台词: %s\n", scene.Script)
			if scene.Audio != "" {
				buf += fmt.Sprintf("音效: %s\n", scene.Audio)
			}
		}
	}

	return buf
}

// integrateScript 整合最终脚本
func (d *Director) integrateScript(s *schema.VideoScript, r *schema.ResearcherOutput, iterations int) *schema.VideoScript {
	if s == nil {
		s = &schema.VideoScript{}
	}

	// 计算总时长和镜头数
	totalDuration := 0
	for _, scene := range s.Scenes {
		totalDuration += scene.Duration
	}

	if s.Metadata.TotalDuration == 0 {
		s.Metadata.TotalDuration = totalDuration
	}
	s.Metadata.SceneCount = len(s.Scenes)
	s.Metadata.Iterations = iterations

	// 添加使用的来源
	if r != nil && len(r.Sources) > 0 {
		s.Metadata.ResearchUsed = r.Sources
	}

	return s
}

// ParseVideoScript 解析视频脚本 JSON
func ParseVideoScript(jsonStr string) (*schema.VideoScript, error) {
	var script schema.VideoScript
	if err := json.Unmarshal([]byte(jsonStr), &script); err != nil {
		return nil, err
	}
	return &script, nil
}

// ============================================================================
// 并行生成器（可选，用于更复杂的并行工作流）
// ============================================================================

type ParallelDirector struct {
	director *Director
	wg       sync.WaitGroup
	mu       sync.Mutex
	results  map[string]interface{}
}

// NewParallelDirector 创建并行导演
func NewParallelDirector(d *Director) *ParallelDirector {
	return &ParallelDirector{
		director: d,
		results:  make(map[string]interface{}),
	}
}

// ParallelResearch 并行执行多个研究任务
func (p *ParallelDirector) ParallelResearch(ctx context.Context, themes []string) map[string]*schema.ResearcherOutput {
	results := make(map[string]*schema.ResearcherOutput)
	var mu sync.Mutex

	for _, theme := range themes {
		p.wg.Add(1)
		go func(t string) {
			defer p.wg.Done()

			result, _ := p.director.researcher.Research(ctx, &schema.ResearcherInput{
				Theme:   t,
				Aspects: []string{"最新动态", "数据统计", "案例分析"},
			})

			mu.Lock()
			results[t] = result
			mu.Unlock()
		}(theme)
	}

	p.wg.Wait()
	return results
}
