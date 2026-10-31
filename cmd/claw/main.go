// cmd/claw/main.go
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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
	promptPtr := flag.String("prompt", "", "可选的首条任务，执行后继续多轮对话")
	workDirPtr := flag.String("dir", ".", "Agent 运行的工作区目录路径 (默认为当前目录)")
	sessionPtr := flag.String("session", "cli_default_session", "指定当前进程中的会话 ID")
	flag.Parse()

	// 加载 .env 并选择启动模式。
	mode, err := loadMode()
	if err != nil {
		log.Fatal(err)
	}
	if mode == "feishu" && (strings.TrimSpace(os.Getenv("FEISHU_APP_ID")) == "" || strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET")) == "") {
		log.Fatal("飞书模式需要设置 FEISHU_APP_ID 和 FEISHU_APP_SECRET")
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
	spanName := "CLI.Chat"
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
	// mcp工具
	// if err := mcpManager.Register(ctx, registry); err != nil {
	// 	log.Fatalf("注册 MCP 工具失败: %v", err)
	// }

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

	if err := runCLI(ctx, eng, sess, reporter, os.Stdin, os.Stdout, *promptPtr); err != nil {
		log.Printf("❌ 对话中断: %v", err)
	}
}

// runCLI 复用引擎和会话，逐条处理终端输入。
func runCLI(ctx context.Context, eng *engine.AgentEngine, sess *sessionpkg.Session, reporter engine.Reporter, input io.Reader, output io.Writer, initialPrompt string) error {
	reader := bufio.NewReader(input)
	prompt := strings.TrimSpace(initialPrompt)
	fmt.Fprintln(output, "\n💬 已进入多轮对话，输入任务后按回车；输入 /exit 或 /quit 退出。")

	for {
		endOfInput := false
		if prompt == "" {
			fmt.Fprint(output, "你> ")
			line, err := reader.ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return errors.New("读取终端输入失败，请检查输入来源")
			}
			endOfInput = errors.Is(err, io.EOF)
			prompt = strings.TrimSpace(line)
		}

		if prompt == "/exit" || prompt == "/quit" {
			fmt.Fprintln(output, "👋 对话已结束。")
			return nil
		}

		if prompt != "" {
			start := time.Now()
			sess.Append(schema.Message{Role: schema.RoleUser, Content: prompt})
			if err := eng.Run(ctx, sess, reporter); err != nil {
				fmt.Fprintf(output, "\n❌ 本轮任务执行失败: %v\n", err)
			} else {
				fmt.Fprintf(output, "✨ 本轮完成，耗时: %v\n", time.Since(start))
			}
			fmt.Fprintf(output, "💰 会话累计消耗: $%.6f | Token: Input %d, Output %d\n\n",
				sess.TotalCostCNY, sess.TotalPromptTokens, sess.TotalCompletionTokens)
		}

		if endOfInput {
			fmt.Fprintln(output, "\n👋 对话已结束。")
			return nil
		}
		prompt = ""
	}
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
