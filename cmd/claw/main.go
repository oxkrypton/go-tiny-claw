// cmd/claw/main.go
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/oxkrypton/go-tiny-claw/internal/background"
	"github.com/oxkrypton/go-tiny-claw/internal/engine"
	"github.com/oxkrypton/go-tiny-claw/internal/feishu"
	"github.com/oxkrypton/go-tiny-claw/internal/mcpclient"
	"github.com/oxkrypton/go-tiny-claw/internal/provider"
	"github.com/oxkrypton/go-tiny-claw/internal/schema"
	sessionpkg "github.com/oxkrypton/go-tiny-claw/internal/session"
	"github.com/oxkrypton/go-tiny-claw/internal/skill"
	"github.com/oxkrypton/go-tiny-claw/internal/tools"
	"github.com/oxkrypton/go-tiny-claw/internal/trace"
	"github.com/oxkrypton/go-tiny-claw/internal/usage"
)

func main() {
	// 1. 命令行参数解析
	promptPtr := flag.String("prompt", "", "要交给 Agent 执行的任务描述")
	workDirPtr := flag.String("dir", ".", "Agent 运行的工作区目录路径 (默认为当前目录)")
	sessionPtr := flag.String("session", "cli_default_session", "指定会话 ID，支持断点续传")
	flag.Parse()

	// 在检查命令行任务之前加载 .env，飞书模式无需 -prompt。
	mode, err := loadMode()
	if err != nil {
		log.Fatal(err)
	}
	if mode == "feishu" && (strings.TrimSpace(os.Getenv("FEISHU_APP_ID")) == "" || strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET")) == "") {
		log.Fatal("飞书模式需要设置 FEISHU_APP_ID 和 FEISHU_APP_SECRET")
	}

	if mode == "cli" && *promptPtr == "" {
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
	fmt.Printf("📡 启动模式: %s\n", mode)
	fmt.Println("==================================================")

	// 2. 初始化核心基础服务
	var realProvider provider.LLMProvider
	modelName := "deepseek-v4-flash"
	realProvider = provider.NewOpenAIProvider(modelName)

	// 获取当前进程中的 Session
	sess := sessionpkg.GlobalSessionMgr.GetOrCreate(*sessionPtr, workDir)

	// 【全息追踪装配】：初始化链路追踪 Root Span
	spanName := "CLI.TaskRun"
	if mode == "feishu" {
		spanName = "Feishu.Listen"
	}
	ctx, rootSpan := trace.StartSpan(context.Background(), spanName)
	rootSpan.AddAttribute("Prompt", *promptPtr)
	defer func() {
		rootSpan.EndSpan()
		_ = trace.ExportToFile(rootSpan, workDir, sess.ID)
	}()

	// 【全息监控装配】：用 Cost Tracker 将真实大脑包裹起来
	trackedProvider := usage.NewTracker(realProvider, modelName, sess)

	// 后台任务管理器（主工具表和只读表共享）
	bgManager := background.NewTaskManager(workDir)
	defer bgManager.Cleanup()

	// MCP Server 管理器
	mcpManager, err := mcpclient.NewManager(workDir)
	if err != nil {
		log.Fatalf("初始化 MCP 配置失败: %v", err)
	}
	defer mcpManager.Close()

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
	registry.Register(tools.NewEditFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir, bgManager))
	skill.Register(registry, workDir)
	if err := mcpManager.Register(ctx, registry); err != nil {
		log.Fatalf("注册 MCP 工具失败: %v", err)
	}

	// 4. 初始化核心引擎 (组装器内部会自动加载 Composer, Compactor, Recovery, Reminders)
	eng := engine.NewAgentEngine(trackedProvider, registry, false)

	//注册 SubAgent 功能
	registry.Register(tools.NewSubagentTool(eng, readOnlyRegistry, reporter))

	if mode == "feishu" {
		bot := feishu.NewFeishuBot(eng, sess)
		fmt.Println("📨 飞书机器人启动中，连接成功后可在飞书中发送任务。")
		if err := bot.Start(ctx); err != nil {
			log.Fatalf("飞书机器人启动失败: %v", err)
		}
		return
	}

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

// loadMode 从当前目录的 .env 加载配置，已有环境变量优先。
func loadMode() (string, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("读取 .env 配置失败，请检查文件权限和配置格式")
	}

	mode := strings.ToLower(strings.TrimSpace(os.Getenv("mode")))
	switch mode {
	case "", "cli":
		return "cli", nil
	case "feishu":
		return "feishu", nil
	default:
		return "", errors.New("mode 配置无效，请设置为 cli 或 feishu")
	}
}
