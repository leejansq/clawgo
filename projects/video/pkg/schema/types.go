/*
 * Video Script Generation - Schema Types
 * 视频脚本生成的数据结构定义
 */

package schema

import "time"

// ============================================================================
// 用户请求和响应
// ============================================================================

// VideoScriptRequest 视频脚本生成请求
type VideoScriptRequest struct {
	Theme          string `json:"theme"`            // 视频主题
	TargetAudience string `json:"audience"`         // 目标受众
	Duration       int    `json:"duration"`         // 总时长（秒）
	Language       string `json:"language"`         // 语言，默认中文
	StrictDuration bool   `json:"strict_duration"`   // 是否严格遵守时长要求
	HumanFeedback string `json:"human_feedback"`   // 人工反馈修改意见
	Images         string `json:"images"`           // 图片资产，格式：图片1: ./static/nv.png,图片2: ./static/nan.png
}

// GenerateResponse 生成响应
type GenerateResponse struct {
	SessionKey string `json:"session_key"`
	Status     string `json:"status"` // processing, completed, failed
}

// GenerationResult 最终生成结果
type GenerationResult struct {
	SessionKey  string       `json:"session_key"`
	Status      string       `json:"status"`
	FinalScript *VideoScript `json:"final_script,omitempty"`
	SubResults  []SubResult  `json:"sub_results,omitempty"`
	Error       string       `json:"error,omitempty"`
}

// ============================================================================
// 视频脚本结构 - 分镜头格式
// ============================================================================

// VideoScript 视频脚本结构（分镜头格式）
type VideoScript struct {
	Title        string     `json:"title"`        // 视频标题
	Introduction string     `json:"introduction"` // 开场介绍（可选）
	GlobalStyle  string     `json:"global_style"` // 全局视觉风格（所有资产必须统一遵循）
	Scenes       []Scene    `json:"scenes"`       // 分镜头列表
	Assets       []Asset    `json:"assets"`       // 全局资产（人物/道具等，用于生成图片）
	Metadata     ScriptMeta `json:"metadata"`     // 元数据
}

// Asset 资产（人物、道具等，用于生成图片）
type Asset struct {
	Name        string `json:"name"`        // 资产名称（如：科学家、实验室、AI机器人）
	Type        string `json:"type"`        // 类型：character/人物, prop/道具, background/背景, other/其他
	Description string `json:"description"` // 资产描述（用于生成图片）
	Prompt      string `json:"prompt"`      // 文生图提示词（正面）
	Negative    string `json:"negative"`    // 负面提示词（如：低质量、变形、水印等）
}

// Scene 分镜头（用于即梦生成，每镜最长15秒）
type Scene struct {
	Index          int      `json:"index"`           // 镜头序号（从1开始）
	Duration       int      `json:"duration"`        // 时长（秒），最大15
	Description    string   `json:"description"`     // 镜头描述（远景/中景/近景等）
	Script         string   `json:"script"`          // 台词/旁白
	Visual         string   `json:"visual"`          // 画面描述（运镜、专业术语）
	Audio          string   `json:"audio"`           // 音效/音乐建议
	CameraMove     string   `json:"camera_move"`     // 运镜方式（推/拉/摇/移/跟/升降等）
	Style          string   `json:"style"`           // 风格/质感（如：写实、赛博朋克、水墨、像素风等）
	TextEffect     string   `json:"text_effect"`     // 文字动画效果（如：淡入淡出、打字机、闪烁等）
	LightEffect    string   `json:"light_effect"`    // 光效（如：柔光、逆光、光斑、光剑等）
	NegativePrompt string   `json:"negative_prompt"` // 负面描述（防崩神器，如：模糊、变形、低质量等）
	Assets         []string `json:"assets"`          // 引用资产名称列表（如：["科学家", "实验室"]）
}

// ScriptMeta 脚本元数据
type ScriptMeta struct {
	TotalDuration int      `json:"total_duration"` // 总时长
	SceneCount    int      `json:"scene_count"`    // 镜头数量
	CreatedAt     string   `json:"created_at"`
	Iterations    int      `json:"iterations"`
	ResearchUsed  []string `json:"research_sources"`
}

// ============================================================================
// 子智能体结果
// ============================================================================

// SubResult 子智能体执行结果
type SubResult struct {
	AgentType string    `json:"agent_type"`
	Output    string    `json:"output"`
	Timestamp time.Time `json:"timestamp"`
}

// ============================================================================
// 子智能体输入/输出结构
// ============================================================================

// ScriptwriterInput 编剧智能体输入
type ScriptwriterInput struct {
	Theme          string `json:"theme"`
	TargetAudience string `json:"target_audience"`
	Duration       int    `json:"duration"`
	ResearchData   string `json:"research_data"`
	Iteration      int    `json:"iteration"`
	PreviousScript string `json:"previous_script,omitempty"`
	CriticFeedback string `json:"critic_feedback,omitempty"`
	HumanFeedback  string `json:"human_feedback,omitempty"` // 人工反馈
}

// ScriptwriterOutput 编剧智能体输出
type ScriptwriterOutput struct {
	Script *VideoScript `json:"script"`
	Error  string       `json:"error,omitempty"`
}

// ResearcherInput 研究员智能体输入
type ResearcherInput struct {
	Theme   string   `json:"theme"`
	Aspects []string `json:"aspects"` // 需要研究的方面
	Images  string   `json:"images"`  // 图片资产，格式：图片1: ./static/nv.png,图片2: ./static/nan.png
}

// ResearcherOutput 研究员智能体输出
type ResearcherOutput struct {
	Overview        string   `json:"overview"`           // 主题概述和目标人群画像
	AudienceInsights []string `json:"audience_insights"`  // 目标人群的喜好和痛点
	KeyData          []string `json:"key_data"`           // 关键数据点
	CaseStudies      []Case   `json:"case_studies"`       // 有说服力的案例
	Trends           []string `json:"trends"`             // 最新动态/趋势
	FunFacts         []string `json:"fun_facts"`          // 有趣的事实
	Sources          []string `json:"sources"`            // 来源URL
	ImageInsights    []string `json:"image_insights"`     // 基于图片的人物/场景描述补充
	Error            string   `json:"error,omitempty"`
}

// Case 案例
type Case struct {
	Title string `json:"title"`
	Desc  string `json:"desc"`
	URL   string `json:"url,omitempty"`
}

// CriticInput 评论家智能体输入
type CriticInput struct {
	Script         string `json:"script"`
	Theme          string `json:"theme"`
	Duration       int    `json:"duration"`        // 目标视频时长（秒）
	StrictDuration bool   `json:"strict_duration"` // 是否严格遵守时长
}

// CriticOutput 评论家输出
type CriticOutput struct {
	Approved  bool         `json:"approved"`
	Scores    CriticScores `json:"scores"`
	Strengths []string     `json:"strengths"`
	Issues    []string     `json:"issues"`
	Feedback  string       `json:"feedback"`
}

// CriticScores 各维度评分
type CriticScores struct {
	Structure int `json:"structure"` // 结构 1-10
	Script    int `json:"script"`    // 台词 1-10
	Visual    int `json:"visual"`    // 画面 1-10
	Camera    int `json:"camera"`    // 运镜 1-10
	Pacing    int `json:"pacing"`    // 节奏 1-10
	Effects   int `json:"effects"`   // 效果（风格/光效/文字特效）1-10
	Spatial   int `json:"spatial"`   // 空间结构与比例 1-10
}

// ============================================================================
// 导演智能体结构
// ============================================================================

// DirectorTask 导演任务
type DirectorTask struct {
	Request    *VideoScriptRequest
	Researcher *ResearcherOutput
	Script     *VideoScript
	Critic     *CriticOutput
	Iteration  int
}

// DirectorDecision 导演决策
type DirectorDecision struct {
	Action     string `json:"action"`     // research, write, review, revise, finalize
	AgentType  string `json:"agent_type"` // director, scriptwriter, researcher, critic
	TaskDesc   string `json:"task_desc"`
	WaitResult bool   `json:"wait_result"` // 是否等待结果
}
