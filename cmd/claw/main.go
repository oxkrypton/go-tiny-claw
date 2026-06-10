// cmd/claw/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/oxkrypton/go-tiny-claw/internal/background"
	"github.com/oxkrypton/go-tiny-claw/internal/usage"
	"github.com/oxkrypton/go-tiny-claw/internal/engine"
	"github.com/oxkrypton/go-tiny-claw/internal/provider"
	"github.com/oxkrypton/go-tiny-claw/internal/schema"
	sessionpkg "github.com/oxkrypton/go-tiny-claw/internal/session"
	"github.com/oxkrypton/go-tiny-claw/internal/tools"
	"github.com/oxkrypton/go-tiny-claw/internal/trace"
)

func main() {
	// 1. 命令行参数解析
	promptPtr := flag.String("prompt", "", "要交给 Agent 执行的任务描述")
	workDirPtr := flag.String("dir", ".", "Agent 运行的工作区目录路径 (默认为当前目录)")
	sessionPtr := flag.String("session", "cli_default_session", "指定会话 ID，支持断点续传")
	flag.Parse()

	if *promptPtr == "" {
		fmt.Println("用法: go-tiny-claw -prompt \"你的任务描述\" [-dir /path/to/workdir] [-session session_id]")
		os.Exit(1)
	}

	// 解析工作区绝对路径
	workDir, err := filepath.Abs(*workDirPtr)
	if err != nil {
		log.Fatalf("解析工作区路径失败: %v", err)
	}

	fmt.Println("==================================================")
	fmt.Printf("🚀 启动 go-tiny-claw ...\n")
	fmt.Printf("📁 锁定工作区: %s\n", workDir)
	fmt.Println("==================================================")

	// 2. 初始化核心基础服务
	var realProvider provider.LLMProvider
	modelName := "deepseek-v4-flash"
	realProvider = provider.NewOpenAIProvider(modelName)

	// 获取持久化 Session
	sess := sessionpkg.GlobalSessionMgr.GetOrCreate(*sessionPtr, workDir)

	// 【全息监控装配】：用 Cost Tracker 将真实大脑包裹起来
	trackedProvider := usage.NewTracker(realProvider, modelName, sess)

	// 后台任务管理器（主工具表和只读表共享）
	bgManager := background.NewTaskManager(workDir)
	defer bgManager.Cleanup()

	// 子智能体只准备受限的只读注册表
	readOnlyRegistry := tools.NewRegistry()
	readOnlyRegistry.Register(tools.NewReadFileTool(workDir))
	readOnlyRegistry.Register(tools.NewBashTool(workDir, bgManager))

	// 5. 初始化终端输出器
	reporter := engine.NewTerminalReporter()

	// 3. 初始化工具与执行层
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir, bgManager))
	registry.Register(tools.NewEditFileTool(workDir))

	// 在 CLI 模式下，我们默认开启 YOLO 模式（全权信任本地执行），
	// 因此这里暂时不挂载 Feishu 审批 Middleware。

	// 4. 初始化核心引擎 (组装器内部会自动加载 Composer, Compactor, Recovery, Reminders)
	// 开启 EnableThinking = true
	eng := engine.NewAgentEngine(trackedProvider, registry, true)
	registry.Register(tools.NewSubagentTool(eng, readOnlyRegistry, reporter))
	registry.Register(tools.NewSkillTool(workDir))

	// 【全息追踪装配】：初始化链路追踪 Root Span
	ctx, rootSpan := trace.StartSpan(context.Background(), "CLI.TaskRun")
	rootSpan.AddAttribute("Prompt", *promptPtr)
	defer func() {
		rootSpan.EndSpan()
		_ = trace.ExportToFile(rootSpan, workDir, sess.ID)
	}()

	fmt.Printf("\n🎯 收到任务: %s\n\n", *promptPtr)

	// 将用户的 Prompt 压入 Session 记忆
	sess.Append(schema.Message{Role: schema.RoleUser, Content: *promptPtr})

	// 6. 发起冲锋：启动 Main Loop！
	err = eng.Run(ctx, sess, reporter)
	if err != nil {
		log.Fatalf("\n💥 引擎运行崩溃: %v", err)
	}

	fmt.Println("\n==================================================")
	fmt.Printf("✨ 任务结束。总耗时: %v\n", time.Since(rootSpan.StartTime))
	fmt.Printf("💰 Session 累计消耗: $%.6f | Token: Input %d, Output %d\n",
		sess.TotalCostCNY, sess.TotalPromptTokens, sess.TotalCompletionTokens)
	fmt.Println("==================================================")
}
