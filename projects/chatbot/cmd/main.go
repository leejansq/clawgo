/*
 * Chat Bot Management System - Main Entry Point
 * 启动主 Agent管理器和服务
 */

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"

	"github.com/leejansq/clawgo/projects/chatbot/internal/chat"
	"github.com/leejansq/clawgo/projects/chatbot/internal/manager"
	"github.com/leejansq/clawgo/projects/chatbot/internal/server"
	"github.com/leejansq/clawgo/projects/chatbot/internal/store"
)

func main() {
	// 命令行参数
	addr := flag.String("addr", "127.0.0.1:18888", "Server address")
	dbPath := flag.String("db", "./chatbot.db", "SQLite database path")
	staticDir := flag.String("static", "", "Static files directory")
	flag.Parse()

	// 确保数据库目录存在
	dbDir := filepath.Dir(*dbPath)
	if dbDir != "." && dbDir != "" {
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			log.Fatalf("Failed to create database directory: %v", err)
		}
	}

	// 初始化 SQLite 存储
	dbStore, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer dbStore.Close()

	log.Printf("📦 SQLite database: %s", *dbPath)

	// 初始化管理器
	mgr := manager.NewManager(dbStore)

	var chatHandler *chat.ChatHandler

	cm, err := newChatModel(context.Background())
	if err != nil {
		log.Fatalf("Failed to newChatModel: %v", err)
	}
	adapter := chat.NewEinoChatModelAdapter(cm)
	chatHandler = chat.NewChatHandler(mgr, adapter)

	// 初始化服务器
	srv := server.NewServer(mgr, chatHandler, *staticDir)

	// 启动服务器
	log.Println("🤖 Chat Bot Management System")
	log.Println("================================")
	log.Printf("🌐 HTTP API: http://%s", *addr)
	log.Printf("📝 Web Console: http://%s/chat/{{sessionKey}}", *addr)
	log.Println("================================")

	if err := srv.Start(*addr); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func newChatModel(ctx context.Context) (model.ToolCallingChatModel, error) {
	if os.Getenv("MODEL_TYPE") == "ark" {
		return ark.NewChatModel(ctx, &ark.ChatModelConfig{
			APIKey:  os.Getenv("ARK_API_KEY"),
			Model:   os.Getenv("ARK_MODEL"),
			BaseURL: os.Getenv("ARK_BASE_URL"),
		})
	}
	return openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		Model:   os.Getenv("OPENAI_MODEL"),
		BaseURL: os.Getenv("OPENAI_BASE_URL"),
		ByAzure: os.Getenv("OPENAI_BY_AZURE") == "true",
	})
}
