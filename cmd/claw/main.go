// cmd/claw/main.go
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/oxkrypton/go-tiny-claw/internal/engine"
	"github.com/oxkrypton/go-tiny-claw/internal/feishu"
	"github.com/oxkrypton/go-tiny-claw/internal/provider"
	"github.com/oxkrypton/go-tiny-claw/internal/schema"
	"github.com/oxkrypton/go-tiny-claw/internal/tools"
)

func main() {
	//获取工作区
	workDir, _ := os.Getwd()
	workDir += "/testdata"

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
	eng := engine.NewAgentEngine(llmProvider, registry, false)

	sessionID := "test_001"
	sess := engine.GlobalSessionMgr.GetOrCreate(sessionID, workDir)
	sess.Append(schema.Message{Role: schema.RoleUser, Content: ""})

	// 5. 通过 WebSocket 长连接启动飞书 Bot（阻塞）
	log.Println("go-tiny-claw 飞书长连接模式启动中...")
	bot := feishu.NewFeishuBot(eng, *sess)

	registry.Use(func(ctx context.Context, call schema.ToolCall) (bool, string) {
		argsStr := string(call.Arguments)

		// 检查是否命中高危特征库
		if !feishu.IsDangerousCommand(call.Name, argsStr) {
			return true, ""
		}

		taskID := call.ID// 使用大模型生成的唯一 ToolCallID 作为 TaskID

		return feishu.GlovalApprovalMgr.WaitForApproval(
			taskID,
			call.Name,
			argsStr,
			bot.Reporter(),
			5*time.Minute,
		)
	})

	if err := bot.Start(context.Background()); err != nil {
		log.Fatalf("飞书长连接启动失败: %v", err)
	}
}
