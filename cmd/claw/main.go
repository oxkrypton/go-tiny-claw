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

	// 设定测试任务：测试 agent 对已有文件的修改能力
	prompt := `
请帮我完成以下两个任务：

任务 1：修改 testdata/big_config.yaml
- 将 database section 中的 host 从 "localhost" 改为 "db.production.internal"，port 从 3306 改为 5432。
- 修改完成后确认修改正确。

任务 2：修改 testdata/nested.py
- 将 process_batch 方法中最内层 try 块里的 except Exception as e 改为 except (ValueError, TypeError) as e。
- 修改完成后确认修改正确。
`

	err := eng.Run(context.Background(), prompt)
	if err != nil {
		log.Fatalf("引擎运行崩溃: %v", err)
	}
}