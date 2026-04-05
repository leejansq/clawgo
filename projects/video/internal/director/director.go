/*
 * Director Agent - 导演智能体
 * 负责协调编剧、研究员、评论家三个子智能体的工作
 */

package director

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/leejansq/clawgo/internal/session"
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
	mgr          *manager.Manager
	researcher   *researcher.Researcher
	scriptwriter *scriptwriter.Scriptwriter
	critic       *critic.Critic

	// session stores for branch isolation
	rootSessionStore         session.SessionStore // 根 session（图片洞察、研究资料）
	scriptwriterSessionStore session.SessionStore // 编剧分支 session
	criticSessionStore       session.SessionStore // 评论家分支 session（每次迭代重新创建）
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
		mgr:          mgr,
		researcher:   researcher.NewResearcher(mgr),
		scriptwriter: scriptwriter.NewScriptwriter(mgr),
		critic:       critic.NewCritic(mgr),
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

	// ========== 第一阶段：创建根 session 并搜集资料 ==========

	// 创建根 session
	d.rootSessionStore = session.NewSessionStore()
	rootSessionID, err := d.rootSessionStore.CreateSession(".")
	if err != nil {
		return nil, fmt.Errorf("failed to create root session: %w", err)
	}
	fmt.Printf("[Director] Created root session: %s\n", rootSessionID)

	// 0. 如果有图片，先解析图片获取描述

	if req.Images != "" {
		fmt.Printf("开始解析图片: %s\n", req.Images)
		err = d.ParseImages(ctx, req.Images)
		if err != nil {
			fmt.Printf("Warning: failed to parse images: %v\n", err)
		}
	}

	// 1. 研究员搜集资料
	researchInput := &schema.ResearcherInput{
		Theme:   req.Theme,
		Aspects: []string{"最新AI视频动态", "主题相关话题数据统计", "主题相关视频案例分析", "视频热度"},
	}

	d.mgr.SetSessionStore(d.rootSessionStore)
	researchResult, err := d.researcher.Research(ctx, researchInput)
	if err != nil {
		return nil, fmt.Errorf("research failed: %w", err)
	}
	fmt.Printf("-----------> %#v\n", researchResult)

	// 2. 将研究资料写入根 session
	researchData := d.formatResearchData(researchResult)
	d.rootSessionStore.AppendMessage("system", "【研究资料】\n"+researchData)

	// ========== 第二阶段：创建编剧分支（独立的 session store）==========

	// 从根 session 创建编剧分支
	d.scriptwriterSessionStore, err = d.createBranchFromRoot("scriptwriter")
	if err != nil {
		return nil, fmt.Errorf("failed to create scriptwriter branch: %w", err)
	}
	fmt.Printf("[Director] Created scriptwriter branch\n")

	// 整理研究资料
	researchData = d.formatResearchData(researchResult)

	// 第二步：编剧生成初稿（在编剧分支上）
	// 设置 manager 使用编剧分支
	d.mgr.SetSessionStore(d.scriptwriterSessionStore)

	currentScript, err := d.scriptwriter.WriteScript(ctx, &schema.ScriptwriterInput{
		Theme:          req.Theme,
		TargetAudience: req.TargetAudience,
		Duration:       req.Duration,
		ResearchData:   researchData,
		Iteration:      1,
		HumanFeedback:  req.HumanFeedback,
	})
	if err != nil {
		return nil, fmt.Errorf("script generation failed: %w", err)
	}

	// 第三步：评论家审查（可能需要迭代）
	d.criticSessionStore, err = d.createBranchFromRoot("critic")
	if err != nil {
		return nil, fmt.Errorf("failed to create critic branch: %w", err)
	}
	fmt.Printf("[Director] Created critic branch\n")
	iteration := 0
	var lastCriticResult *schema.CriticOutput

	for iteration < maxIterations {
		iteration++

		// 为评论家创建独立的 session 分支（每次迭代重新从根创建）

		// 设置 manager 使用评论家分支
		d.mgr.SetSessionStore(d.criticSessionStore)

		criticInput := &schema.CriticInput{
			Script:         d.formatScriptForReview(currentScript),
			Theme:          req.Theme,
			Duration:       req.Duration,
			StrictDuration: req.StrictDuration,
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

		// 恢复编剧分支（评论家分支不影响编剧分支）
		d.mgr.SetSessionStore(d.scriptwriterSessionStore)

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
			HumanFeedback:  req.HumanFeedback,
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

	if len(r.AudienceInsights) > 0 {
		buf += "【目标人群洞察】\n"
		for _, insight := range r.AudienceInsights {
			buf += fmt.Sprintf("- %s\n", insight)
		}
		buf += "\n"
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

	fmt.Printf("formatScriptForReview: %#v\n", s.Scenes)
	var buf string

	if s.Title != "" {
		buf += fmt.Sprintf("【标题】%s\n", s.Title)
	}
	if s.Introduction != "" {
		buf += fmt.Sprintf("【开场介绍】%s\n", s.Introduction)
	}
	if s.GlobalStyle != "" {
		buf += fmt.Sprintf("【全局视觉风格】%s\n", s.GlobalStyle)
	}

	// 显示全局资产（人物/道具等）
	if len(s.Assets) > 0 {
		buf += "\n【资产（文生图）】\n"
		for _, asset := range s.Assets {
			buf += fmt.Sprintf("[%s] %s\n", asset.Type, asset.Name)
			buf += fmt.Sprintf("  描述: %s\n", asset.Description)
			buf += fmt.Sprintf("  提示词: %s\n", asset.Prompt)
			if asset.Negative != "" {
				buf += fmt.Sprintf("  负面提示词: %s\n", asset.Negative)
			}
		}
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
			if scene.Style != "" {
				buf += fmt.Sprintf("风格: %s\n", scene.Style)
			}
			if scene.TextEffect != "" {
				buf += fmt.Sprintf("文字特效: %s\n", scene.TextEffect)
			}
			if scene.LightEffect != "" {
				buf += fmt.Sprintf("光效: %s\n", scene.LightEffect)
			}
			if scene.NegativePrompt != "" {
				buf += fmt.Sprintf("负面描述: %s\n", scene.NegativePrompt)
			}
			// 显示引用的资产
			if len(scene.Assets) > 0 {
				buf += fmt.Sprintf("使用资产: %s\n", strings.Join(scene.Assets, ", "))
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

// ParseImages 通过 reactAgent 读取并解析图片，获取图片描述
func (d *Director) ParseImages(ctx context.Context, imagesStr string) error {
	if imagesStr == "" {
		return nil
	}

	paths := parseImagePaths(imagesStr)
	fmt.Printf("解析到的图片路径: %v\n", paths)
	if len(paths) == 0 {
		return nil
	}

	// 构建读取图片的 prompt
	var pathList string
	for i, p := range paths {
		pathList += fmt.Sprintf("图片%d路径：%s\n", i+1, p)
	}
	readPrompt := fmt.Sprintf(`请使用工具读取和理解以下图片，获取图片描述。

图片列表：
%s

请逐个调用工具获取这些图片信息，然后在回复中以以下格式返回图片描述：
{
  "image_descriptions": [
    "图片1: 描述内容",
    "图片2: 描述内容"
  ]
}`, pathList)

	// 调用 LLM，让 reactAgent 处理工具调用
	descOutput, err := d.mgr.GenerateWithTools(ctx, "", readPrompt)
	if err != nil {
		return fmt.Errorf("failed to generate image descriptions: %w", err)
	}

	d.rootSessionStore.AppendMessage("system", descOutput)
	return nil
}

// parseImagePaths 解析图片路径字符串
// 格式：图片1: ./static/nv.png,图片2: ./static/nan.png
func parseImagePaths(imagesStr string) []string {
	var paths []string
	// 按逗号分割
	parts := strings.Split(imagesStr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		// 查找冒号后的路径
		if idx := strings.Index(part, ":"); idx >= 0 && idx < len(part)-1 {
			path := strings.TrimSpace(part[idx+1:])
			// 简单验证是否是图片路径
			if strings.HasSuffix(strings.ToLower(path), ".png") ||
				strings.HasSuffix(strings.ToLower(path), ".jpg") ||
				strings.HasSuffix(strings.ToLower(path), ".jpeg") ||
				strings.HasSuffix(strings.ToLower(path), ".gif") ||
				strings.HasSuffix(strings.ToLower(path), ".webp") {
				paths = append(paths, path)
			}
		}
	}
	return paths
}

// buildImageSection 构建图片描述说明
func buildImageSection(imageInsights []string) string {
	if len(imageInsights) == 0 {
		return ""
	}

	imageSection := "【图片资产描述】\n"
	for _, insight := range imageInsights {
		imageSection += insight + "\n"
	}
	imageSection += "请将上述图片描述作为人物/场景参考，补充到研究报告的 image_insights 字段中。\n"
	return imageSection
}

// getCurrentBranchID 获取当前分支的 leaf ID
func (d *Director) getCurrentBranchID() string {
	branch := d.rootSessionStore.GetBranch()
	if len(branch) == 0 {
		return ""
	}
	return branch[len(branch)-1].GetID()
}

// createBranchFromRoot 从根 session 创建新分支（新的 session store）
func (d *Director) createBranchFromRoot(branchName string) (session.SessionStore, error) {
	// 获取根 session 的当前 leaf
	branch := d.rootSessionStore.GetBranch()
	if len(branch) == 0 {
		return nil, fmt.Errorf("root session has no entries")
	}
	rootLeafID := branch[len(branch)-1].GetID()

	fmt.Printf("[Director] Root session file: %s\n", d.rootSessionStore.GetSessionFile())

	// 使用 CreateBranch 创建新分支，不会修改根 session
	newStore, err := d.rootSessionStore.CreateBranch(rootLeafID)
	if err != nil {
		return nil, err
	}

	// 获取新分支 session 的信息
	newSessionID := newStore.GetSessionID()
	newSessionFile := newStore.GetSessionFile()
	fmt.Printf("[Director] Created %s session: %s (file: %s)\n", branchName, newSessionID, newSessionFile)

	// 在新分支中添加分支标记
	newStore.AppendMessage("system", fmt.Sprintf("【%s 分支】由根 session 创建\n", branchName))

	// 打印新分支的 session 历史
	d.printBranchHistory(branchName, newStore)

	return newStore, nil
}

// printBranchHistory 打印分支的 session 历史
func (d *Director) printBranchHistory(branchName string, store session.SessionStore) {
	branch := store.GetBranch()
	if len(branch) == 0 {
		fmt.Printf("[Director] %s branch is empty\n", branchName)
		return
	}

	fmt.Printf("[Director] %s branch history (%d entries):\n", branchName, len(branch))
	for i, entry := range branch {
		var role, content string
		if me, ok := entry.(*session.MessageEntry); ok {
			role = me.Message.Role
			content = me.Message.Content
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			fmt.Printf("  [%d] %s: %s\n", i, role, content)
		} else {
			fmt.Printf("  [%d] entry type: %s\n", i, entry.GetType())
		}
	}
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
				Aspects: []string{"最新AI视频动态", "主题相关话题数据统计", "主题相关视频案例分析", "视频热度"},
			})

			mu.Lock()
			results[t] = result
			mu.Unlock()
		}(theme)
	}

	p.wg.Wait()
	return results
}
