/*
 * Researcher Agent - 研究员智能体
 * 负责搜集和整理相关资料
 * 通过 LLM + Tool 的方式，让 LLM 自主决定何时调用 web_search
 */

package researcher

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/leejansq/clawgo/projects/video/internal/manager"
	"github.com/leejansq/clawgo/projects/video/pkg/schema"
)

// Researcher 研究员智能体
type Researcher struct {
	mgr *manager.Manager
}

// NewResearcher 创建研究员智能体
func NewResearcher(mgr *manager.Manager) *Researcher {
	return &Researcher{
		mgr: mgr,
	}
}

// Research 执行研究任务
// 通过 LLM + Tool 的方式，让 LLM 自主决定何时调用 web_search
func (r *Researcher) Research(ctx context.Context, input *schema.ResearcherInput) (*schema.ResearcherOutput, error) {
	// 构建 LLM 输入，让 LLM 自己决定是否调用 web_search
	aspects := ""
	if len(input.Aspects) > 0 {
		aspects = "\n重点关注以下方面：\n"
		for _, a := range input.Aspects {
			aspects += fmt.Sprintf("- %s\n", a)
		}
	}

	userInput := fmt.Sprintf(`请为视频主题"%s"搜集研究资料。

%s

请使用 web_search 工具搜索相关信息，搜集：
1. 主题概述和背景
2. 最新资讯和动态
3. 关键数据和统计
4. 典型案例
5. 发展趋势

搜索完成后，请以 JSON 格式返回结构化的研究结果：
{
  "overview": "主题概述（100字以内）",
  "key_data": ["关键数据点1", "关键数据点2"],
  "case_studies": [{"title": "案例标题", "desc": "案例描述", "url": "来源URL"}],
  "trends": ["最新趋势1", "最新趋势2"],
  "fun_facts": ["有趣的事实1", "有趣的事实2"],
  "sources": ["来源URL1", "来源URL2"]
}
`, input.Theme, aspects)

	// 调用 LLM（支持工具调用）
	output, err := r.mgr.GenerateWithTools(ctx, getResearcherPrompt(), userInput)
	if err != nil {
		return &schema.ResearcherOutput{
			Overview: "",
			Error:    err.Error(),
		}, nil
	}

	// 解析输出
	var result schema.ResearcherOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		// 如果不是 JSON，返回纯文本
		result.Overview = output
	}

	return &result, nil
}

// getResearcherPrompt 获取研究员 prompt
func getResearcherPrompt() string {
	return `你是视频脚本生成系统的研究员智能体，负责为编剧搜集和整理相关资料。

你的职责：
1. 理解视频主题和目标受众
2. 使用 web_search 工具搜索相关信息
3. 搜集同时切合主题和受众喜好相关的信息，以及趋势信息
4. 收集视频以及影视案例，分析其内容、镜头和风格
5. 将搜集到的信息结构化整理

重要：你有权使用 web_search 工具来获取最新信息。当需要了解某个主题的相关信息时，请主动调用 web_search。

输出格式要求：
请以 JSON 格式返回研究结果，包含以下字段：
{
  "overview": "主题概述（100字以内）",
  "key_data": ["关键数据点1", "关键数据点2", ...],
  "case_studies": [{"title": "案例标题", "desc": "案例描述", "url": "来源URL"}, ...],
  "trends": ["最新趋势1", "最新趋势2", ...],
  "fun_facts": ["有趣的事实1", "有趣的事实2", ...],
  "sources": ["来源URL1", "来源URL2", ...]
}

请保持信息的准确性和来源可靠性。`
}
