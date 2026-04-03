/*
 * Terminal UI Package - 终端交互界面
 * 参考 claw-code 设计的彩色终端 UI
 */

package terminal

import (
	"os"

	"github.com/leejansq/clawgo/projects/touliu/internal/types"
)

// isTerminal 检查是否在终端环境
func isTerminal() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	// Check if stdout is a terminal (char device)
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// Decision 用户决策类型
type Decision int

const (
	Approve Decision = iota // 批准执行
	Revise                  // 请求修订
	Reject                  // 拒绝
	Quit                    // 退出
)

func (d Decision) String() string {
	switch d {
	case Approve:
		return "approve"
	case Revise:
		return "revise"
	case Reject:
		return "reject"
	case Quit:
		return "quit"
	default:
		return "unknown"
	}
}

// PermissionMode 权限模式
type PermissionMode int

const (
	ReadOnly PermissionMode = iota // 只读模式，只允许查看
	Prompt                         // 每次确认（默认）
	AutoApproveSafe               // 自动批准安全操作
	Bypass                         // 跳过确认
)

func (m PermissionMode) String() string {
	switch m {
	case ReadOnly:
		return "只读"
	case Prompt:
		return "确认"
	case AutoApproveSafe:
		return "自动批准"
	case Bypass:
		return "跳过"
	default:
		return "未知"
	}
}

// ColorTheme 终端颜色主题
type ColorTheme struct {
	Heading      string // 标题颜色
	Emphasis     string // 强调颜色
	Strong       string // 粗体颜色
	InlineCode   string // 内联代码颜色
	Link         string // 链接颜色
	Quote        string // 引用颜色
	TableBorder  string // 表格边框颜色
	Success      string // 成功颜色
	Error        string // 错误颜色
	Warning      string // 警告颜色
	Spinner      string // 加载动画颜色
	SpinnerDone  string // 完成颜色
	SpinnerFail  string // 失败颜色
	PromptSymbol string // 提示符号
}

func DefaultTheme() ColorTheme {
	return ColorTheme{
		Heading:      "\033[36m", // Cyan
		Emphasis:     "\033[35m", // Magenta
		Strong:       "\033[33m", // Yellow
		InlineCode:   "\033[32m", // Green
		Link:         "\033[34m", // Blue
		Quote:         "\033[90m", // DarkGrey
		TableBorder:  "\033[36m", // Cyan
		Success:      "\033[32m", // Green
		Error:        "\033[31m", // Red
		Warning:      "\033[33m", // Yellow
		Spinner:      "\033[34m", // Blue
		SpinnerDone:  "\033[32m", // Green
		SpinnerFail:  "\033[31m", // Red
		PromptSymbol: "\033[36m▶\033[0m",
	}
}

func (c ColorTheme) Reset() string {
	return "\033[0m"
}

func (c ColorTheme) HeadingStyle(text string) string {
	return c.Heading + text + c.Reset()
}

func (c ColorTheme) SuccessStyle(text string) string {
	return c.Success + text + c.Reset()
}

func (c ColorTheme) ErrorStyle(text string) string {
	return c.Error + text + c.Reset()
}

func (c ColorTheme) WarningStyle(text string) string {
	return c.Warning + text + c.Reset()
}

func (c ColorTheme) StrongStyle(text string) string {
	return c.Strong + text + c.Reset()
}

func (c ColorTheme) CodeStyle(text string) string {
	return c.InlineCode + text + c.Reset()
}

func (c ColorTheme) EmphasisStyle(text string) string {
	return c.Emphasis + text + c.Reset()
}

// TerminalUI 终端 UI 接口
type TerminalUI interface {
	// ReadDecision 读取用户决策
	// 返回决策类型和可选的反馈文本
	ReadDecision(prompt string) (Decision, string)

	// DisplayPlan 以结构化方式展示投放方案
	DisplayPlan(plan *types.CampaignPlan, platform string)

	// DisplaySection 显示一个区块
	DisplaySection(title, content string)

	// DisplayJSON 以格式化 JSON 显示数据
	DisplayJSON(data interface{})

	// StartSpinner 开始显示加载动画
	StartSpinner(label string)

	// StopSpinner 停止加载动画
	StopSpinner(label string, success bool)

	// DisplayHelp 显示帮助信息
	DisplayHelp()

	// IsTerminal 检查是否在终端环境
	IsTerminal() bool
}

// DefaultTerminalUI 默认终端 UI 实现
type DefaultTerminalUI struct {
	theme    ColorTheme
	permMode PermissionMode
}

// NewTerminalUI 创建默认终端 UI
func NewTerminalUI(mode PermissionMode) *DefaultTerminalUI {
	return &DefaultTerminalUI{
		theme:    DefaultTheme(),
		permMode: mode,
	}
}

// IsTerminal 检查是否在终端环境
func (t *DefaultTerminalUI) IsTerminal() bool {
	return isTerminal()
}
