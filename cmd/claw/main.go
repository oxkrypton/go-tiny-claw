// cmd/claw/main.go
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/oxkrypton/go-tiny-claw/internal/engine"
	"github.com/oxkrypton/go-tiny-claw/internal/provider"
	"github.com/oxkrypton/go-tiny-claw/internal/schema"
	"github.com/oxkrypton/go-tiny-claw/internal/tools"
)

func main() {
	//获取工作区
	workDir, _ := os.Getwd()

	// 1. 初始化真实的 Provider大脑
	// 这里你可以任意切换 NewClaudeProvider 或 NewOpenAIProvider，效果完全一致！
	llmProvider := provider.NewOpenAIProvider("deepseek-v4-flash")

	// 2. 注入伪造的工具注册表
	registry := tools.NewRegistry()

	// 3.挂载 4 大基础工具
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	// 4. 实例化并运行引擎
	eng := engine.NewAgentEngine(llmProvider, registry, true)

	//便于测试的终端输出器
	reporter := engine.NewTerminalReporter()

	sessionID := "test_oom_protection_001"
	sess := engine.GlobalSessionMgr.GetOrCreate(sessionID, workDir)

	// 通过命令行参数接收用户的 prompt
	prompt := flag.String("prompt", "", "要交给 Agent 执行的任务描述")
	flag.Parse()

	log.Printf("\n>>> 🚀 收到指令: %s\n", *prompt)
	
	sess.Append(schema.Message{Role: schema.RoleUser, Content: *prompt})

	err := eng.Run(context.Background(), sess, reporter)
	if err != nil {
		log.Fatal("fail to start engine: %v", err)
	}

	// 5. 通过 WebSocket 长连接启动飞书 Bot（阻塞）
	// log.Println("go-tiny-claw 飞书长连接模式启动中...")
	// if err := feishu.NewFeishuBot(eng).Start(context.Background()); err != nil {
	// 	log.Fatalf("飞书长连接启动失败: %v", err)
	// }
}
