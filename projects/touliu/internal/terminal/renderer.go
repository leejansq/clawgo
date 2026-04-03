/*
 * Terminal Renderer - 终端渲染器
 * 支持 Markdown 渲染和结构化表格
 */

package terminal

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/leejansq/clawgo/projects/touliu/internal/types"
)

// BoxDrawing 框线字符
const (
	BoxTopLeft     = "╔"
	BoxTopRight    = "╗"
	BoxBottomLeft  = "╚"
	BoxBottomRight = "╝"
	BoxHorizontal  = "═"
	BoxVertical    = "║"
	BoxCross       = "╬"
	BoxLeftT       = "╠"
	BoxRightT      = "╣"
	BoxTopT        = "╦"
	BoxBottomT     = "╩"
)

const BoxWidth = 64

// Renderer 终端渲染器
type Renderer struct {
	Theme ColorTheme
}

// NewRenderer 创建渲染器
func NewRenderer() *Renderer {
	return &Renderer{
		Theme: DefaultTheme(),
	}
}

// RenderBox 渲染一个带标题的盒子
func (r *Renderer) RenderBox(title string, lines []string) string {
	var b strings.Builder
	width := BoxWidth

	// 顶部边框
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxTopLeft)
	for i := 0; i < width-2; i++ {
		b.WriteString(BoxHorizontal)
	}
	b.WriteString(BoxTopRight)
	b.WriteString(r.Theme.Reset())
	b.WriteString("\n")

	// 标题行
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxVertical)
	titleLen := len(title)
	padding := (width - 2 - titleLen) / 2
	for i := 0; i < padding; i++ {
		b.WriteString(" ")
	}
	b.WriteString(r.Theme.StrongStyle(title))
	for i := 0; i < width-2-padding-titleLen; i++ {
		b.WriteString(" ")
	}
	b.WriteString(BoxVertical)
	b.WriteString(r.Theme.Reset())
	b.WriteString("\n")

	// 分隔线
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxLeftT)
	for i := 0; i < width-2; i++ {
		b.WriteString(BoxHorizontal)
	}
	b.WriteString(BoxRightT)
	b.WriteString(r.Theme.Reset())
	b.WriteString("\n")

	// 内容行
	for _, line := range lines {
		b.WriteString(r.Theme.Heading)
		b.WriteString(BoxVertical)
		b.WriteString(r.Theme.Reset())
		b.WriteString("  ")
		b.WriteString(line)
		padding := width - 4 - len(line)
		for i := 0; i < padding; i++ {
			b.WriteString(" ")
		}
		b.WriteString(r.Theme.Heading)
		b.WriteString(BoxVertical)
		b.WriteString(r.Theme.Reset())
		b.WriteString("\n")
	}

	// 底部边框
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxBottomLeft)
	for i := 0; i < width-2; i++ {
		b.WriteString(BoxHorizontal)
	}
	b.WriteString(BoxBottomRight)
	b.WriteString(r.Theme.Reset())

	return b.String()
}

// RenderPlan 渲染投放方案
func (r *Renderer) RenderPlan(plan *types.CampaignPlan, platform string) string {
	var b strings.Builder
	width := BoxWidth

	// 格式化方案行
	lines := r.formatPlanLines(plan, platform)

	// 顶部边框
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxTopLeft)
	for i := 0; i < width-2; i++ {
		b.WriteString(BoxHorizontal)
	}
	b.WriteString(BoxTopRight)
	b.WriteString(r.Theme.Reset())
	b.WriteString("\n")

	// 标题行
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxVertical)
	b.WriteString(r.Theme.Reset())
	title := "  投放方案详情  "
	b.WriteString(r.Theme.StrongStyle(title))
	padding := width - 2 - len(title)
	for i := 0; i < padding; i++ {
		b.WriteString(" ")
	}
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxVertical)
	b.WriteString(r.Theme.Reset())
	b.WriteString("\n")

	// 分隔线
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxLeftT)
	for i := 0; i < width-2; i++ {
		b.WriteString(BoxHorizontal)
	}
	b.WriteString(BoxRightT)
	b.WriteString(r.Theme.Reset())
	b.WriteString("\n")

	// 内容行
	for _, line := range lines {
		b.WriteString(r.Theme.Heading)
		b.WriteString(BoxVertical)
		b.WriteString(r.Theme.Reset())
		b.WriteString(" ")
		b.WriteString(line)
		padding := width - 3 - len(stripANSICodes(line))
		for i := 0; i < padding; i++ {
			b.WriteString(" ")
		}
		b.WriteString(r.Theme.Heading)
		b.WriteString(BoxVertical)
		b.WriteString(r.Theme.Reset())
		b.WriteString("\n")
	}

	// 底部边框
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxBottomLeft)
	for i := 0; i < width-2; i++ {
		b.WriteString(BoxHorizontal)
	}
	b.WriteString(BoxBottomRight)
	b.WriteString(r.Theme.Reset())

	return b.String()
}

// formatPlanLines 格式化方案为行列表
func (r *Renderer) formatPlanLines(plan *types.CampaignPlan, platform string) []string {
	var lines []string

	// 基本信息行
	roiStr := fmt.Sprintf("%.2f", plan.ROIExpectation)
	lines = append(lines, fmt.Sprintf("  产品: %s  %s  预期ROI: %s",
		r.Theme.StrongStyle(plan.Product),
		r.Theme.PromptSymbol,
		r.Theme.WarningStyle(roiStr)))

	lines = append(lines, fmt.Sprintf("  平台: %s  %s  风险: %s",
		r.Theme.CodeStyle(platform),
		r.Theme.PromptSymbol,
		r.Theme.WarningStyle(plan.RiskAssessment)))

	// 分隔线用空行代替
	lines = append(lines, "")

	// 定向设置
	lines = append(lines, r.Theme.EmphasisStyle("  ▶ 定向设置"))
	lines = append(lines, fmt.Sprintf("    年龄: %s  性别: %s  设备: %s",
		r.Theme.CodeStyle(fmt.Sprintf("%d-%d岁", plan.Targeting.AgeRange[0], plan.Targeting.AgeRange[1])),
		r.Theme.CodeStyle(plan.Targeting.Gender),
		r.Theme.CodeStyle(strings.Join(plan.Targeting.DeviceTypes, ", "))))

	if len(plan.Targeting.Locations) > 0 {
		lines = append(lines, fmt.Sprintf("    地域: %s",
			r.Theme.CodeStyle(strings.Join(plan.Targeting.Locations, ", "))))
	}
	if len(plan.Targeting.Interests) > 0 {
		lines = append(lines, fmt.Sprintf("    兴趣: %s",
			r.Theme.CodeStyle(strings.Join(plan.Targeting.Interests, ", "))))
	}

	lines = append(lines, "")

	// 预算配置
	lines = append(lines, r.Theme.EmphasisStyle("  ▶ 预算配置"))
	lines = append(lines, fmt.Sprintf("    出价: %s ¥%.2f  %s  日预算: ¥%.0f  %s  总预算: ¥%.0f",
		r.Theme.CodeStyle(plan.Bid.BidType),
		plan.Bid.BidAmount,
		r.Theme.PromptSymbol,
		plan.Bid.DailyBudget,
		r.Theme.PromptSymbol,
		plan.Bid.TotalBudget))

	lines = append(lines, "")

	// 创意简报
	if plan.CreativeBrief != "" {
		lines = append(lines, r.Theme.EmphasisStyle("  ▶ 创意简报"))
		lines = append(lines, fmt.Sprintf("    %s", plan.CreativeBrief))
		lines = append(lines, "")
	}

	// 优化建议
	if len(plan.Recommendations) > 0 {
		lines = append(lines, r.Theme.EmphasisStyle("  ▶ 优化建议"))
		for i, rec := range plan.Recommendations {
			lines = append(lines, fmt.Sprintf("    %d. %s", i+1, rec))
		}
	}

	return lines
}

// RenderJSON 渲染 JSON 数据
func (r *Renderer) RenderJSON(data interface{}) string {
	var b strings.Builder
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return r.Theme.ErrorStyle(fmt.Sprintf("JSON 渲染失败: %v", err))
	}

	jsonStr := string(jsonBytes)
	lines := strings.Split(jsonStr, "\n")

	width := BoxWidth

	// 顶部边框
	b.WriteString(r.Theme.InlineCode)
	b.WriteString(BoxTopLeft)
	for i := 0; i < width-2; i++ {
		b.WriteString(BoxHorizontal)
	}
	b.WriteString(BoxTopRight)
	b.WriteString(r.Theme.Reset())
	b.WriteString("\n")

	// 标题行
	b.WriteString(r.Theme.InlineCode)
	b.WriteString(BoxVertical)
	b.WriteString(r.Theme.Reset())
	b.WriteString("  JSON ")
	padding := width - 4 - 5
	for i := 0; i < padding; i++ {
		b.WriteString(" ")
	}
	b.WriteString(r.Theme.InlineCode)
	b.WriteString(BoxVertical)
	b.WriteString(r.Theme.Reset())
	b.WriteString("\n")

	// 分隔线
	b.WriteString(r.Theme.InlineCode)
	b.WriteString(BoxLeftT)
	for i := 0; i < width-2; i++ {
		b.WriteString(BoxHorizontal)
	}
	b.WriteString(BoxRightT)
	b.WriteString(r.Theme.Reset())
	b.WriteString("\n")

	// JSON 内容
	for _, line := range lines {
		b.WriteString(r.Theme.InlineCode)
		b.WriteString(BoxVertical)
		b.WriteString(r.Theme.Reset())
		b.WriteString(" ")
		b.WriteString(r.Theme.InlineCode)
		b.WriteString(line)
		b.WriteString(r.Theme.Reset())
		padding := width - 3 - len(stripANSICodes(line))
		for i := 0; i < padding; i++ {
			b.WriteString(" ")
		}
		b.WriteString(r.Theme.InlineCode)
		b.WriteString(BoxVertical)
		b.WriteString(r.Theme.Reset())
		b.WriteString("\n")
	}

	// 底部边框
	b.WriteString(r.Theme.InlineCode)
	b.WriteString(BoxBottomLeft)
	for i := 0; i < width-2; i++ {
		b.WriteString(BoxHorizontal)
	}
	b.WriteString(BoxBottomRight)
	b.WriteString(r.Theme.Reset())

	return b.String()
}

// RenderHelp 渲染帮助信息
func (r *Renderer) RenderHelp() string {
	var b strings.Builder
	width := BoxWidth

	commands := [][]string{
		{"命令", "说明"},
		{"/yes, /y", "批准当前方案，开始执行"},
		{"/revise, /r", "请求修订方案"},
		{"/no, /n", "拒绝方案并退出"},
		{"/quit, /q", "直接退出"},
		{"/json", "显示方案原始 JSON"},
		{"/help, /h", "显示帮助信息"},
	}

	// 顶部边框
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxTopLeft)
	for i := 0; i < width-2; i++ {
		b.WriteString(BoxHorizontal)
	}
	b.WriteString(BoxTopRight)
	b.WriteString(r.Theme.Reset())
	b.WriteString("\n")

	// 标题行
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxVertical)
	b.WriteString(r.Theme.Reset())
	title := "  可用命令  "
	b.WriteString(r.Theme.StrongStyle(title))
	padding := width - 2 - len(title)
	for i := 0; i < padding; i++ {
		b.WriteString(" ")
	}
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxVertical)
	b.WriteString(r.Theme.Reset())
	b.WriteString("\n")

	// 分隔线
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxLeftT)
	for i := 0; i < width-2; i++ {
		b.WriteString(BoxHorizontal)
	}
	b.WriteString(BoxRightT)
	b.WriteString(r.Theme.Reset())
	b.WriteString("\n")

	// 表头
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxVertical)
	b.WriteString(r.Theme.Reset())
	b.WriteString(" ")
	b.WriteString(r.Theme.StrongStyle("命令"))
	padding = width - 4 - 4
	for i := 0; i < padding; i++ {
		b.WriteString(" ")
	}
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxVertical)
	b.WriteString(r.Theme.Reset())
	b.WriteString(" ")
	b.WriteString(r.Theme.StrongStyle("说明"))
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxVertical)
	b.WriteString(r.Theme.Reset())
	b.WriteString("\n")

	// 分隔线
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxLeftT)
	for i := 0; i < width-2; i++ {
		b.WriteString(BoxHorizontal)
	}
	b.WriteString(BoxRightT)
	b.WriteString(r.Theme.Reset())
	b.WriteString("\n")

	// 内容行
	for i, row := range commands {
		if i == 0 {
			continue // 跳过表头
		}
		b.WriteString(r.Theme.Heading)
		b.WriteString(BoxVertical)
		b.WriteString(r.Theme.Reset())
		b.WriteString(" ")
		b.WriteString(r.Theme.CodeStyle(row[0]))
		padding = width - 4 - len(stripANSICodes(row[0]))
		for i := 0; i < padding; i++ {
			b.WriteString(" ")
		}
		b.WriteString(r.Theme.Heading)
		b.WriteString(BoxVertical)
		b.WriteString(r.Theme.Reset())
		b.WriteString(" ")
		b.WriteString(row[1])
		b.WriteString(r.Theme.Heading)
		b.WriteString(BoxVertical)
		b.WriteString(r.Theme.Reset())
		b.WriteString("\n")
	}

	// 底部边框
	b.WriteString(r.Theme.Heading)
	b.WriteString(BoxBottomLeft)
	for i := 0; i < width-2; i++ {
		b.WriteString(BoxHorizontal)
	}
	b.WriteString(BoxBottomRight)
	b.WriteString(r.Theme.Reset())

	return b.String()
}

// stripANSICodes 去除 ANSI 转义序列
func stripANSICodes(s string) string {
	var result strings.Builder
	inEscape := false
	for _, ch := range s {
		if ch == '\033' {
			inEscape = true
			continue
		}
		if inEscape && ch == 'm' {
			inEscape = false
			continue
		}
		if !inEscape {
			result.WriteRune(ch)
		}
	}
	return result.String()
}
