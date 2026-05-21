// cmd/claw/main.go
package main

import (
	"context"
	"log"
	"os"

	"github.com/oxkrypton/go-tiny-claw/internal/engine"
	"github.com/oxkrypton/go-tiny-claw/internal/provider"
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

	// 4. 实例化并运行引擎，开启 EnableThinking = true (开启慢思考阶段)
	eng := engine.NewAgentEngine(llmProvider, registry, workDir, false)
	
	//便于测试的终端输出器
	reporter := engine.NewTerminalReporter()
	prompt := `
	1.使用git diff检查当前项目的改动
	2.完善.claw/skills/git-workflow/SKILL.md的git工作流skill
	3.生成合适的commit信息,将这次所有改动提交并push上去
	`
	err := eng.Run(context.Background(), prompt, reporter)
	if err != nil {
		log.Fatalf("引擎运行崩溃: %v", err)
	}

	// 5. 通过 WebSocket 长连接启动飞书 Bot（阻塞）
	// log.Println("go-tiny-claw 飞书长连接模式启动中...")
	// if err := feishu.NewFeishuBot(eng).Start(context.Background()); err != nil {
	// 	log.Fatalf("飞书长连接启动失败: %v", err)
	// }
}
