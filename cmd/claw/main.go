// cmd/claw/main.go
package main

import (
	"context"
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

	// 3.初始化真实的 Tool Registry
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	// 4. 实例化并运行引擎
	eng := engine.NewAgentEngine(llmProvider, registry)

	//便于测试的终端输出器
	reporter := engine.NewTerminalReporter()

	sessionID := "test_oom_protection_001"
	sess := engine.GlobalSessionMgr.GetOrCreate(sessionID, workDir)

	// 发起一个会导致读取大文件的恶意任务
	prompt := ` 
	请帮我执行以下三个步骤： 
	1. 使用 bash 执行 echo "开始排查日志" 
	2. 使用 read_file 工具读取当前目录下的巨大文件 testdata/mock_log.txt 
	3. 使用 bash 执行 date 命令获取当前时间，并告诉我任务全部完成。 
	`

	sess.Append(schema.Message{Role: schema.RoleUser, Content: prompt})

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
