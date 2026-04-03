/*
 * Line Editor - 行编辑器
 * 支持 slash 命令自动补全
 */

package terminal

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// SlashCommand slash 命令定义
type SlashCommand struct {
	Name        string
	Aliases     []string
	Description string
}

// AllSlashCommands 所有可用的 slash 命令
var AllSlashCommands = []SlashCommand{
	{Name: "/yes", Aliases: []string{"/y"}, Description: "批准方案"},
	{Name: "/revise", Aliases: []string{"/r", "/change", "/c"}, Description: "修订方案"},
	{Name: "/no", Aliases: []string{"/n", "/reject"}, Description: "拒绝方案"},
	{Name: "/quit", Aliases: []string{"/q", "/exit"}, Description: "退出"},
	{Name: "/json", Aliases: []string{"/raw"}, Description: "显示 JSON"},
	{Name: "/help", Aliases: []string{"/h", "/?"}, Description: "显示帮助"},
}

// LineEditor 行编辑器
type LineEditor struct {
	reader     *bufio.Reader
	theme      ColorTheme
	commands   []SlashCommand
}

// NewLineEditor 创建行编辑器
func NewLineEditor() *LineEditor {
	return &LineEditor{
		reader:   bufio.NewReader(os.Stdin),
		theme:    DefaultTheme(),
		commands: AllSlashCommands,
	}
}

// ReadLine 读取一行输入
func (e *LineEditor) ReadLine(prompt string) (string, error) {
	fmt.Print("> ")

	line, err := e.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(line), nil
}

// ReadDecisionWithSlash 读取带 slash 命令解析的输入
// 返回 (Decision, feedback string, error)
func (e *LineEditor) ReadDecisionWithSlash() (Decision, string, error) {
	input, err := e.ReadLine("")
	if err != nil {
		return Quit, "", err
	}

	input = strings.TrimSpace(input)

	// 空输入
	if input == "" {
		return Revise, "", nil
	}

	// 转换为小写进行匹配
	lowerInput := strings.ToLower(input)

	// 检查是否是 slash 命令
	if strings.HasPrefix(input, "/") {
		return e.parseSlashCommand(lowerInput)
	}

	// 检查快捷键
	switch lowerInput {
	case "y", "yes":
		return Approve, "", nil
	case "n", "no":
		return Reject, "", nil
	case "c", "change":
		// c 单独使用表示修订，需要读取反馈
		return Revise, "", nil
	case "q", "quit":
		return Quit, "", nil
	}

	// 默认当作修订反馈处理
	return Revise, input, nil
}

// parseSlashCommand 解析 slash 命令
func (e *LineEditor) parseSlashCommand(cmd string) (Decision, string, error) {
	// 去除前导 /
	cmd = strings.TrimPrefix(cmd, "/")

	// 检查是否带有反馈 (格式: /revise feedback text)
	parts := strings.SplitN(cmd, " ", 2)
	baseCmd := parts[0]
	var feedback string
	if len(parts) > 1 {
		feedback = parts[1]
	}

	for _, command := range e.commands {
		if command.Name == "/"+baseCmd {
			return e.commandToDecision(command.Name, feedback)
		}
		for _, alias := range command.Aliases {
			if alias == "/"+baseCmd {
				return e.commandToDecision(command.Name, feedback)
			}
		}
	}

	// 未知命令，返回修订并把原输入作为反馈
	return Revise, "未知命令: /" + baseCmd, nil
}

// commandToDecision 将命令转换为决策
func (e *LineEditor) commandToDecision(cmd string, feedback string) (Decision, string, error) {
	switch cmd {
	case "/yes":
		return Approve, "", nil
	case "/revise":
		return Revise, feedback, nil
	case "/no":
		return Reject, "", nil
	case "/quit":
		return Quit, "", nil
	case "/json":
		// 特殊处理：返回特殊值表示显示 JSON
		return Revise, "__SHOW_JSON__", nil
	case "/help":
		// 特殊处理：返回特殊值表示显示帮助
		return Revise, "__SHOW_HELP__", nil
	default:
		return Revise, "", nil
	}
}

// ReadFeedback 读取单行反馈 (按回车结束)
func (e *LineEditor) ReadFeedback(instruction string) (string, error) {
	fmt.Println()
	fmt.Println("------------------------------------------")
	fmt.Printf("  %s\n", instruction)
	fmt.Println("------------------------------------------")
	fmt.Print("  > ")

	line, err := e.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(line), nil
}

// CompleteCommand 补全命令 (返回匹配的命令列表)
func (e *LineEditor) CompleteCommand(prefix string) []string {
	var matches []string
	for _, cmd := range e.commands {
		if strings.HasPrefix(cmd.Name, prefix) {
			matches = append(matches, cmd.Name)
		}
		for _, alias := range cmd.Aliases {
			if strings.HasPrefix(alias, prefix) {
				matches = append(matches, alias)
			}
		}
	}
	return matches
}

// DisplayPrompt 显示确认提示
func (e *LineEditor) DisplayPrompt(revisionCount int) {
	fmt.Println()
	fmt.Println("========================================")
	if revisionCount > 0 {
		fmt.Printf("  %s 第 %d 轮修订后确认\n", e.theme.WarningStyle("⏸"), revisionCount)
	} else {
		fmt.Printf("  %s 确认投放方案\n", e.theme.WarningStyle("⏸"))
	}
	fmt.Println("========================================")

	// 显示命令提示
	fmt.Println()
	fmt.Printf("  命令: %s - 批准  %s - 修订  %s - 拒绝  %s - JSON  %s - 帮助\n",
		e.theme.CodeStyle("/yes"),
		e.theme.CodeStyle("/revise"),
		e.theme.CodeStyle("/no"),
		e.theme.CodeStyle("/json"),
		e.theme.CodeStyle("/help"))
	fmt.Println()
	fmt.Print("  > ")
}

// ClearLine 清除当前行
func (e *LineEditor) ClearLine() {
	fmt.Print("\r" + strings.Repeat(" ", 80) + "\r")
}

// PrintAligned 打印对齐的内容
func (e *LineEditor) PrintAligned(label, value string, width int) {
	padding := width - len(stripANSICodes(label)) - len(stripANSICodes(value))
	if padding < 0 {
		padding = 0
	}
	fmt.Printf("  %s%s %s\n", label, strings.Repeat(" ", padding), value)
}
