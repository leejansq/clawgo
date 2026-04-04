/*
 * Scriptwriter Agent - 编剧智能体
 * 负责将抽象创意转化为分镜头视频脚本
 */

package scriptwriter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/leejansq/clawgo/projects/video/internal/manager"
	"github.com/leejansq/clawgo/projects/video/pkg/schema"
)

// Scriptwriter 编剧智能体
type Scriptwriter struct {
	mgr *manager.Manager
}

// NewScriptwriter 创建编剧智能体
func NewScriptwriter(mgr *manager.Manager) *Scriptwriter {
	return &Scriptwriter{
		mgr: mgr,
	}
}

// WriteScript 编写视频脚本
func (s *Scriptwriter) WriteScript(ctx context.Context, input *schema.ScriptwriterInput) (*schema.VideoScript, error) {
	// 设置默认值
	if input.Duration == 0 {
		input.Duration = 120
	}
	if input.Iteration == 0 {
		input.Iteration = 1
	}

	// 调用 manager 执行
	script, err := s.mgr.ExecuteScriptwriter(ctx, input)
	if err != nil {
		return nil, err
	}

	// 设置元数据
	if script.Metadata.TotalDuration == 0 {
		script.Metadata.TotalDuration = input.Duration
		script.Metadata.CreatedAt = fmt.Sprintf("%d", input.Iteration)
		script.Metadata.Iterations = input.Iteration
	}

	return script, nil
}

// parseVideoScript 解析视频脚本 JSON
func parseVideoScript(output string) (*schema.VideoScript, error) {
	// 移除 <think>...</think> 标签（Claude 等模型的思考过程）
	cleaned := removeThinkTags(output)

	// 尝试直接解析
	var script schema.VideoScript
	if err := json.Unmarshal([]byte(cleaned), &script); err == nil && len(script.Scenes) > 0 {
		return &script, nil
	}

	// 尝试提取 ```json 代码块（取最后一个，因为 LLM 经常先生成一个草稿再生成最终版本）
	jsonBlocks := extractAllJsonBlocks(cleaned)
	for i := len(jsonBlocks) - 1; i >= 0; i-- {
		if err := json.Unmarshal([]byte(jsonBlocks[i]), &script); err == nil && len(script.Scenes) > 0 {
			return &script, nil
		}
	}

	// 如果都不是，创建一个简化版本
	script = schema.VideoScript{
		Title: "视频脚本",
	}

	// 尝试从纯文本中解析分镜头
	script.Scenes = parseScenesFromText(cleaned)

	return &script, nil
}

// removeThinkTags 移除 <think>...</think> 标签
func removeThinkTags(s string) string {
	result := ""
	for {
		start := indexOf(s, "<think>")
		if start < 0 {
			result += s
			break
		}
		result += s[:start]
		end := indexOf(s, "</think>")
		if end < 0 {
			break
		}
		s = s[end+8:]
	}
	return result
}

// extractAllJsonBlocks 提取所有 ```json 代码块内容
func extractAllJsonBlocks(s string) []string {
	var blocks []string
	for {
		idx := indexOf(s, "```json")
		if idx < 0 {
			break
		}
		start := idx + 7
		// 跳过可能的换行
		for start < len(s) && (s[start] == '\n' || s[start] == '\r') {
			start++
		}
		end := indexOf(s[start:], "```")
		if end < 0 {
			break
		}
		blocks = append(blocks, s[start:start+end])
		// 继续查找下一个
		s = s[start+end+3:]
	}
	return blocks
}

// parseScenesFromText 从纯文本中解析分镜头
func parseScenesFromText(text string) []schema.Scene {
	var scenes []schema.Scene
	lines := strings.Split(text, "\n")

	currentScene := -1
	var currentLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 检测镜头标记
		if strings.Contains(line, "镜头") || strings.Contains(line, "Scene") || strings.Contains(line, "scene") {
			// 保存上一个镜头
			if currentScene >= 0 && len(currentLines) > 0 {
				scene := parseSceneLines(currentScene, currentLines)
				if scene.Index > 0 {
					scenes = append(scenes, scene)
				}
			}
			// 开始新镜头
			currentScene++
			currentLines = []string{line}
		} else {
			currentLines = append(currentLines, line)
		}
	}

	// 保存最后一个镜头
	if currentScene >= 0 && len(currentLines) > 0 {
		scene := parseSceneLines(currentScene, currentLines)
		if scene.Index > 0 {
			scenes = append(scenes, scene)
		}
	}

	// 如果没有解析到镜头，创建单个镜头
	if len(scenes) == 0 {
		scenes = append(scenes, schema.Scene{
			Index:          1,
			Duration:       15,
			Description:    "中景",
			Script:         text,
			Visual:         "待补充画面描述",
			CameraMove:     "固定",
			Style:          "自然",
			TextEffect:     "无",
			LightEffect:    "自然光",
			NegativePrompt: "模糊、低质量、变形",
			Assets:         []string{},
		})
	}

	// 重新编号
	for i := range scenes {
		scenes[i].Index = i + 1
	}

	return scenes
}

// parseSceneLines 解析单镜头的多行文本
func parseSceneLines(index int, lines []string) schema.Scene {
	scene := schema.Scene{
		Index:          index,
		Duration:       10,
		Description:    "中景",
		Script:         strings.Join(lines, "\n"),
		Visual:         "待补充画面描述",
		CameraMove:     "固定",
		Style:          "自然",
		TextEffect:     "无",
		LightEffect:    "自然光",
		NegativePrompt: "模糊、低质量、变形",
		Assets:         []string{}, // 引用资产名称列表
	}

	// 尝试提取时长
	for _, line := range lines {
		if strings.Contains(line, "秒") || strings.Contains(line, "s") {
			// 简单提取数字
			var dur int
			if _, err := fmt.Sscanf(line, "%d", &dur); err == nil && dur > 0 && dur <= 15 {
				scene.Duration = dur
			}
		}
		// 提取景别
		if strings.Contains(line, "远景") {
			scene.Description = "远景"
		} else if strings.Contains(line, "全景") {
			scene.Description = "全景"
		} else if strings.Contains(line, "中景") {
			scene.Description = "中景"
		} else if strings.Contains(line, "近景") {
			scene.Description = "近景"
		} else if strings.Contains(line, "特写") {
			scene.Description = "特写"
		}
		// 提取运镜
		if strings.Contains(line, "推") {
			scene.CameraMove = "推"
		} else if strings.Contains(line, "拉") {
			scene.CameraMove = "拉"
		} else if strings.Contains(line, "摇") {
			scene.CameraMove = "摇"
		} else if strings.Contains(line, "移") {
			scene.CameraMove = "移"
		} else if strings.Contains(line, "跟") {
			scene.CameraMove = "跟"
		}
	}

	return scene
}

// indexOf 查找子字符串位置
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
