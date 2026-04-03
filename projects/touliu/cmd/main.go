/*
 * 电商投流多 Agent 系统 - 主入口
 * E-Commerce Advertising Multi-Agent System
 *
 * 运行方式:
 *   go run ./projects/touliu/cmd/main.go -product "智能手表" -platform "douyin" -market "一线城市"
 *
 * 环境变量:
 *   OPENAI_API_KEY - OpenAI API Key
 *   OPENAI_MODEL   - 模型名称 (默认 gpt-4o)
 *   MODEL_TYPE     - 模型类型 (openai 或 ark)
 */

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"

	"github.com/leejansq/clawgo/projects/touliu/internal/agents"
)

func main() {
	// 命令行参数
	product := flag.String("product", "智能手表", "产品名称")
	platform := flag.String("platform", "douyin", "投放平台: douyin/weibo/toutiao")
	market := flag.String("market", "一线城市", "目标市场")
	knowledgePath := flag.String("knowledge", "./knowledge", "知识库路径")
	workspacePath := flag.String("workspace", "./workspace", "工作空间路径")
	skillDirs := flag.String("skills", "./skills", "技能目录(逗号分隔)")
	showResult := flag.Bool("result", true, "显示最终结果")
	flag.Parse()

	// 确保目录存在
	absKnowledgePath, _ := filepath.Abs(*knowledgePath)
	absWorkspacePath, _ := filepath.Abs(*workspacePath)
	os.MkdirAll(absKnowledgePath, 0755)
	os.MkdirAll(absWorkspacePath, 0755)

	// 解析技能目录
	var skillDirList []string
	for _, d := range strings.Split(*skillDirs, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			absDir, _ := filepath.Abs(d)
			os.MkdirAll(absDir, 0755)
			skillDirList = append(skillDirList, absDir)
		}
	}

	// 创建上下文，支持取消
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 处理信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n收到中断信号，正在退出...")
		cancel()
	}()

	// 初始化 LLM
	cm, tcm, err := newChatModel(ctx)
	if err != nil {
		log.Fatalf("Failed to create chat model: %v", err)
	}

	// 显示配置信息
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║       电商投流多 Agent 系统 - E-Commerce Ad Multi-Agent      ║")
	fmt.Println("╠════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  产品: %-50s║\n", *product)
	fmt.Printf("║  平台: %-50s║\n", getPlatformDisplay(*platform))
	fmt.Printf("║  市场: %-50s║\n", *market)
	fmt.Printf("║  知识库: %-48s║\n", absKnowledgePath)
	fmt.Printf("║  工作空间: %-47s║\n", absWorkspacePath)
	fmt.Printf("║  技能目录: %-48s║\n", strings.Join(skillDirList, ","))
	fmt.Println("╚════════════════════════════════════════════════════════════╝")

	// 创建协调器
	coord, err := agents.NewCoordinator(&agents.CoordinatorConfig{
		Model:            cm,
		ToolCallingModel: tcm,
		Workspace:       absWorkspacePath,
		KnowledgePath:   absKnowledgePath,
		SkillDirs:        skillDirList,
	})
	if err != nil {
		log.Fatalf("Failed to create coordinator: %v", err)
	}
	defer coord.Stop()

	// 执行工作流
	state, err := coord.Run(ctx, *product, *platform, *market)
	if err != nil {
		log.Fatalf("Workflow failed: %v", err)
	}

	// 显示结果
	if *showResult && state.OverallStatus == "completed" {
		fmt.Println("\n📄 最终工作流状态:")
		fmt.Println(agents.WorkflowStateToJSON(state))
	}
}

// newChatModel 创建聊天模型
func newChatModel(ctx context.Context) (model.ChatModel, model.ToolCallingChatModel, error) {
	modelType := os.Getenv("MODEL_TYPE")
	if modelType == "" {
		modelType = "openai"
	}

	if modelType == "ark" {
		cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
			APIKey:  os.Getenv("ARK_API_KEY"),
			Model:   os.Getenv("ARK_MODEL"),
			BaseURL: os.Getenv("ARK_BASE_URL"),
		})
		if err != nil {
			return nil, nil, err
		}
		return cm, cm, nil
	}

	cm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		Model:   os.Getenv("OPENAI_MODEL"),
		BaseURL: os.Getenv("OPENAI_BASE_URL"),
	})
	if err != nil {
		return nil, nil, err
	}
	return cm, cm, nil
}

// getPlatformDisplay 获取平台显示名称
func getPlatformDisplay(platform string) string {
	switch strings.ToLower(platform) {
	case "douyin":
		return "抖音 (Douyin)"
	case "weibo":
		return "微博 (Weibo)"
	case "toutiao":
		return "头条 (Toutiao)"
	default:
		return platform
	}
}
