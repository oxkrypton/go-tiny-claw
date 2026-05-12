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

	// 1. 初始化真实的 Provider大脑 (指向智谱 GLM-4.5)
	// 这里你可以任意切换 NewZhipuClaudeProvider 或 NewZhipuOpenAIProvider，效果完全一致！
	llmProvider := provider.NewOpenAIProvider("deepseek-v4-flash")

	// 2. 注入伪造的工具注册表
	registry := tools.Registry.GetAvailableTools()

	// 3. 实例化并运行引擎，开启 EnableThinking = true (开启慢思考阶段！)
	eng := engine.NewAgentEngine(llmProvider, registry, workDir, false)

	// 设定测试任务
	prompt := "我想去北京跑步，帮我查查天气适合吗？"

	err := eng.Run(context.Background(), prompt)
	if err != nil {
		log.Fatalf("引擎运行崩溃: %v", err)
	}
}
