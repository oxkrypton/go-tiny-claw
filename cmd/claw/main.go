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
	workDir += "/testdata"

	sessionID := "test_001"
	sess := engine.GlobalSessionMgr.GetOrCreate(sessionID, workDir)

	// 1. 初始化真实的 Provider大脑
	llmProvider := provider.NewOpenAIProvider("deepseek-v4-flash")

	// 注入的工具注册表
	registry := tools.NewRegistry()

	// 子智能体只准备受限的只读注册表
	readOnlyRegistry := tools.NewRegistry()
	readOnlyRegistry.Register(tools.NewReadFileTool(workDir))
	readOnlyRegistry.Register(tools.NewBashTool(workDir))

	reporter := engine.NewTerminalReporter()

	// 实例化并运行引擎
	eng := engine.NewAgentEngine(llmProvider, registry, false)

	// 挂载 5 大基础工具
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))
	//将subagent功能注入
	registry.Register(tools.NewSubagentTool(eng, readOnlyRegistry, reporter))

	prompt := ` 
我需要你在这个遗留项目里，找到那个“核心密码”。 
为了防止污染主上下文，请你务必派出子智能体（spawn_subagent）去执行探索任务。 
你可以让子智能体使用 bash 去查找当前目录（及其所有子目录）下名为 config.txt 的文件。 
子智能体拿到密码向你汇报后，请你亲自使用 write_file 工具，将密码写在根目录的 answer.txt 里。 
`
	log.Println("\n>>> 🚀 启动多智能体协同测试...")
	sess.Append(schema.Message{Role: schema.RoleUser, Content: prompt})
	err := eng.Run(context.Background(), sess, reporter)
	if err != nil {
		log.Fatalf("引擎运行崩溃: %v", err)
	}
}
