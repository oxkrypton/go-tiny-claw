// cmd/claw/main.go
package main

import (
	"context"
	"log"
	"os"

	ctxpkg "github.com/oxkrypton/go-tiny-claw/internal/context"
	"github.com/oxkrypton/go-tiny-claw/internal/engine"
	"github.com/oxkrypton/go-tiny-claw/internal/observability"
	"github.com/oxkrypton/go-tiny-claw/internal/provider"
	"github.com/oxkrypton/go-tiny-claw/internal/schema"
	"github.com/oxkrypton/go-tiny-claw/internal/tools"
)

func main() {
	//获取工作区
	workDir, _ := os.Getwd()
	workDir += "/testdata"

	modelName := "deepseek-v4-flash"

	sessionID := "test_001"
	sess := ctxpkg.GlobalSessionMgr.GetOrCreate(sessionID, workDir)

	// 初始化真实的 Provider大脑
	llmProvider := provider.NewOpenAIProvider(modelName)

	// 用 tracker 将大模型包裹起来
	trackedProvider := observability.NewCostTracker(llmProvider, modelName, sess)

	// 注入的工具注册表
	registry := tools.NewRegistry()

	// 子智能体只准备受限的只读注册表
	readOnlyRegistry := tools.NewRegistry()
	readOnlyRegistry.Register(tools.NewReadFileTool(workDir))
	readOnlyRegistry.Register(tools.NewBashTool(workDir))

	reporter := engine.NewTerminalReporter()

	// 实例化并运行引擎
	eng := engine.NewAgentEngine(trackedProvider, registry, false)

	// 挂载 5 大基础工具
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))
	//将subagent功能注入
	registry.Register(tools.NewSubagentTool(eng, readOnlyRegistry, reporter))
	// skill 动态加载工具（渐进式披露：按需加载完整 SKILL.md）
	registry.Register(tools.NewSkillTool(workDir))

	prompt := `请用 bash 帮我用 date 命令查一下现在的时间.`
	log.Println("\n>>> 🚀 启动测试...")
	sess.Append(schema.Message{Role: schema.RoleUser, Content: prompt})

	err := eng.Run(context.Background(), sess, reporter)
	if err != nil {
		log.Fatalf("引擎运行崩溃: %v", err)
	}

	log.Printf("会话 ID: %s\n", sess.ID)
	log.Printf("总消耗 Input Tokens: %d\n", sess.TotalPromptTokens)
	log.Printf("总消耗 Output Tokens: %d\n", sess.TotalCompletionTokens)
	log.Printf("总计费用 (CNY): ¥%.6f\n", sess.TotalCostCNY)
}
