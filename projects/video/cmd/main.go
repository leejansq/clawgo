/*
 * Video Script Generation - Main Entry Point
 * 视频脚本生成智能体系统主入口（命令行模式）
 */

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"

	"github.com/leejansq/clawgo/projects/video/internal/director"
	"github.com/leejansq/clawgo/projects/video/internal/manager"
	"github.com/leejansq/clawgo/projects/video/internal/tools"
	"github.com/leejansq/clawgo/projects/video/pkg/schema"
)

var (
	modelType = flag.String("model", "openai", "Model type: openai or ark")
	mcpClientSpec = flag.String("mcp", "", "MCP client config (format: name=xxx cmd='command args' env='KEY1=val1,KEY2=val2')")
)

func main() {
	flag.Parse()

	// 初始化 ChatModel (ToolCallingChatModel)
	ctx := context.Background()
	cm, err := newChatModel(ctx)
	if err != nil {
		fmt.Printf("Failed to create chat model: %v\n", err)
		os.Exit(1)
	}

	// 初始化 ToolCallingManager
	mgr := manager.NewToolCallingManager(cm)

	// 注册 MCP 工具（如果指定了 MCP 服务器）
	if *mcpClientSpec != "" {
		mcpClient, err := parseAndConnectMCP(ctx, *mcpClientSpec)
		if err != nil {
			fmt.Printf("Failed to connect to MCP server: %v\n", err)
			os.Exit(1)
		}
		mcpProvider := tools.NewMCPToolProvider(mcpClient)
		mgr.RegisterToolProvider(mcpProvider)
		fmt.Println("MCP web_search tool registered")
	} else {
		// 使用本地 web_search 工具
		webSearchProvider := tools.NewWebSearchToolProvider()
		mgr.RegisterToolProvider(webSearchProvider)
		fmt.Println("Local web_search tool registered")
	}

	// 初始化 Director
	dir := director.NewDirector(mgr)

	// 运行命令行交互
	runCLI(dir)
}

// newChatModel 创建 LLM 模型
func newChatModel(ctx context.Context) (model.ToolCallingChatModel, error) {
	switch *modelType {
	case "ark":
		return ark.NewChatModel(ctx, &ark.ChatModelConfig{
			APIKey:  os.Getenv("ARK_API_KEY"),
			Model:   os.Getenv("ARK_MODEL"),
			BaseURL: os.Getenv("ARK_BASE_URL"),
		})
	case "openai":
		fallthrough
	default:
		return openai.NewChatModel(ctx, &openai.ChatModelConfig{
			APIKey:  os.Getenv("OPENAI_API_KEY"),
			Model:   os.Getenv("OPENAI_MODEL"),
			BaseURL: os.Getenv("OPENAI_BASE_URL"),
		})
	}
}

// ============================================================================
// 命令行交互
// ============================================================================

func runCLI(dir *director.Director) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n========================================")
	fmt.Println("    视频脚本生成智能体系统")
	fmt.Println("    Video Script Generation Agent")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("用法：")
	fmt.Println("  输入视频主题，回车开始生成")
	fmt.Println("  输入 q 或 quit 退出")
	fmt.Println()

	for {
		fmt.Print("📝 请输入视频主题: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Printf("读取输入失败: %v\n", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// 检查退出命令
		if input == "q" || input == "quit" || input == "exit" {
			fmt.Println("再见！")
			break
		}

		// 生成脚本
		generateScript(dir, input, reader)
	}
}

func generateScript(dir *director.Director, theme string, reader *bufio.Reader) {
	// 收集基本信息
	fmt.Println()
	fmt.Println("--- 基本信息收集 ---")

	fmt.Print("  目标受众 (直接回车使用默认值'普通观众'): ")
	audience, _ := reader.ReadString('\n')
	audience = strings.TrimSpace(audience)
	if audience == "" {
		audience = "普通观众"
	}

	fmt.Print("  视频时长(秒，默认120秒，直接回车使用默认值): ")
	durationStr, _ := reader.ReadString('\n')
	durationStr = strings.TrimSpace(durationStr)
	duration := 120
	if durationStr != "" {
		fmt.Sscanf(durationStr, "%d", &duration)
	}

	fmt.Print("  严格时长(true/false，默认false，直接回车使用默认值): ")
	strictStr, _ := reader.ReadString('\n')
	strictStr = strings.TrimSpace(strings.ToLower(strictStr))
	strictDuration := false
	if strictStr == "true" || strictStr == "是" {
		strictDuration = true
	}

	fmt.Println()
	fmt.Println("--- 开始生成视频脚本 ---")
	fmt.Println("  (LLM会根据需要自动调用web_search工具搜集资料)")
	fmt.Println()

	// 构建请求
	req := &schema.VideoScriptRequest{
		Theme:          theme,
		TargetAudience: audience,
		Duration:       duration,
		StrictDuration: strictDuration,
	}

	// 生成
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	startTime := time.Now()
	result, err := dir.Generate(ctx, req)
	elapsed := time.Since(startTime)

	if err != nil {
		fmt.Printf("\n❌ 生成失败: %v\n", err)
		return
	}

	// 打印结果
	printResult(result)

	fmt.Printf("\n⏱️  生成耗时: %v\n", elapsed)

	// 人工反馈环节
	for {
		fmt.Println()
		fmt.Print("是否满意当前脚本？(直接回车或输入y表示满意，输入n表示需要修改): ")
		satisfied, _ := reader.ReadString('\n')
		satisfied = strings.TrimSpace(strings.ToLower(satisfied))

		if satisfied == "" || satisfied == "y" || satisfied == "yes" {
			fmt.Println("\n✅ 脚本已确认，生成完成！")
			return
		}

		if satisfied == "n" || satisfied == "no" {
			fmt.Println("\n请输入您的修改意见（直接回车结束输入，以空行结束）:")
			fmt.Println("--------------------------------------------------------")
			var feedbackLines []string
			for {
				line, _ := reader.ReadString('\n')
				line = strings.TrimRight(line, "\r\n")
				if line == "" {
					break
				}
				feedbackLines = append(feedbackLines, line)
			}
			humanFeedback := strings.Join(feedbackLines, "\n")
			if humanFeedback == "" {
				fmt.Println("未输入修改意见，保留当前版本。")
				return
			}

			fmt.Println("\n收到修改意见，正在重新生成...")
			fmt.Println("--------------------------------------------------------")

			// 带反馈重新生成（创建新的context，避免超时）
			req.HumanFeedback = humanFeedback
			regenCtx, regenCancel := context.WithTimeout(context.Background(), 10*time.Minute)
			startTime = time.Now()
			result, err = dir.Generate(regenCtx, req)
			regenCancel()
			if err != nil {
				fmt.Printf("\n❌ 重新生成失败: %v\n", err)
				return
			}

			// 打印新结果
			printResult(result)
			fmt.Printf("\n⏱️  生成耗时: %v\n", elapsed)
			continue
		}

		fmt.Println("无效输入，请输入 y/n 或直接回车表示满意")
	}
}

func printResult(result *schema.GenerationResult) {
	fmt.Println("\n========================================")
	fmt.Println("          视频脚本生成结果")
	fmt.Println("========================================\n")

	if result.FinalScript != nil {
		script := result.FinalScript
		if script.Title != "" {
			fmt.Printf("【标题】%s\n\n", script.Title)
		}
		if script.Introduction != "" {
			fmt.Printf("【开场介绍】\n%s\n\n", script.Introduction)
		}

		// 打印分镜头
		if len(script.Scenes) > 0 {
			fmt.Println("【分镜头脚本】")
			fmt.Println("----------------------------------------")
			for _, scene := range script.Scenes {
				fmt.Printf("\n📹 镜头 %d (%d秒)\n", scene.Index, scene.Duration)
				fmt.Printf("   景别: %s\n", scene.Description)
				fmt.Printf("   运镜: %s\n", scene.CameraMove)
				fmt.Printf("   画面: %s\n", scene.Visual)
				fmt.Printf("   台词: %s\n", scene.Script)
				if scene.Audio != "" {
					fmt.Printf("   音效: %s\n", scene.Audio)
				}
			}
			fmt.Println("----------------------------------------")
		}

		// 打印元数据
		fmt.Printf("\n总时长: %d秒 | 镜头数: %d\n", script.Metadata.TotalDuration, script.Metadata.SceneCount)
	}

	fmt.Println("\n========================================")
	fmt.Printf("状态: %s\n", result.Status)
	fmt.Println("========================================\n")

	// 如果需要，输出 JSON 格式
	fmt.Println("【JSON格式输出】")
	jsonData, _ := json.MarshalIndent(result.FinalScript, "", "  ")
	fmt.Println(string(jsonData))
}

// ============================================================================
// MCP 客户端解析和连接
// ============================================================================

// parseClientSpec 解析客户端规范字符串
// 格式: name=xxx cmd='command args' env='KEY1=val1,KEY2=val2'
func parseClientSpec(spec string) map[string]string {
	result := make(map[string]string)

	fields := strings.Split(spec, " ")

	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}

		// 查找第一个 = 的位置
		idx := strings.Index(field, "=")
		if idx == -1 {
			continue
		}

		key := strings.TrimSpace(field[:idx])
		value := strings.TrimSpace(field[idx+1:])
		// 去除首尾引号
		value = strings.Trim(value, "\"'")

		if key != "" {
			result[key] = value
		}
	}

	return result
}

// unquote 去除字符串首尾的引号
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// parseAndConnectMCP 解析 MCP 配置并连接
func parseAndConnectMCP(ctx context.Context, spec string) (*tools.MCPClient, error) {
	// 解析键值对
	parts := parseClientSpec(spec)

	var name, cmd string
	var envList []string

	for k, v := range parts {
		switch k {
		case "name":
			name = v
		case "cmd":
			cmd = unquote(v)
		case "env":
			// 解析环境变量，格式: KEY1=val1,KEY2=val2
			envStr := unquote(v)
			envPairs := strings.Split(envStr, ",")
			for _, pair := range envPairs {
				pair = strings.TrimSpace(pair)
				if pair != "" {
					envList = append(envList, pair)
				}
			}
		}
	}

	if name == "" || cmd == "" {
		return nil, fmt.Errorf("invalid MCP client spec: %s (expected format: name=xxx cmd='command' env='KEY=val')", spec)
	}

	fmt.Printf("Connecting to MCP server: %s (cmd: %s, env: %v)\n", name, cmd, envList)

	mcpClient := tools.NewMCPClient(name, cmd, envList)
	if err := mcpClient.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return mcpClient, nil
}
