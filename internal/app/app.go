// Package app 负责启动配置、依赖装配、模式选择和资源生命周期。
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/oxkrypton/go-tiny-claw/internal/cli"
	"github.com/oxkrypton/go-tiny-claw/internal/engine"
	"github.com/oxkrypton/go-tiny-claw/internal/feishu"
	"github.com/oxkrypton/go-tiny-claw/internal/schema"
	"github.com/oxkrypton/go-tiny-claw/internal/trace"
)

// Run 在所选模式中运行应用，资源清理完成后再向调用方返回错误。
func Run(ctx context.Context, options Options, input io.Reader, output io.Writer) (err error) {
	cfg, err := loadConfig(options)
	if err != nil {
		return err
	}

	fmt.Fprintln(output, "==================================================")
	fmt.Fprintln(output, "🚀 启动 go-tiny-claw ...")
	fmt.Fprintf(output, "📁 锁定工作区: %s\n", cfg.workDir)
	fmt.Fprintf(output, "📡 启动模式: %s\n", cfg.mode)
	fmt.Fprintln(output, "==================================================")

	spanName := "CLI.Chat"
	if cfg.mode == modeFeishu {
		spanName = "Feishu.Listen"
	}
	ctx, span := trace.StartSpan(ctx, spanName)
	span.AddAttribute("Prompt", cfg.initialPrompt)
	defer func() {
		span.EndSpan()
		if exportErr := trace.ExportToFile(span, cfg.workDir, cfg.sessionID); exportErr != nil {
			log.Printf("保存运行记录失败: %v", exportErr)
		}
	}()

	// 保留现有子任务的终端输出方式；输出器通过参数传入装配层。
	reporter := engine.NewTerminalReporter()
	rt, err := newRuntime(ctx, cfg, reporter)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, rt.Close()) }()

	switch cfg.mode {
	case modeFeishu:
		bot := feishu.NewFeishuBot(rt.engine, rt.session)
		approve := func(ctx context.Context, call schema.ToolCall) (bool, string) {
			args := string(call.Arguments)
			if !feishu.IsDangerousCommand(call.Name, args) {
				return true, ""
			}
			return feishu.GlovalApprovalMgr.WaitForApproval(
				call.ID, call.Name, args, bot.Reporter(), 5*time.Minute,
			)
		}
		rt.tools.main.Use(approve)
		rt.tools.subagent.Use(approve)
		fmt.Fprintln(output, "📨 飞书机器人启动中，连接成功后可在飞书中发送任务。")
		if err := bot.Start(ctx); err != nil {
			return fmt.Errorf("飞书机器人启动失败: %w", err)
		}
		return nil
	default:
		conversation := cli.New(rt.engine, rt.session, reporter)
		return conversation.Run(ctx, input, output, cfg.initialPrompt)
	}
}
