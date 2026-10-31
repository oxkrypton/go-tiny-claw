package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/oxkrypton/go-tiny-claw/internal/app"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
}

func run(args []string, input io.Reader, output io.Writer) error {
	var options app.Options
	flags := flag.NewFlagSet("go-tiny-claw", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {
		fmt.Fprintln(output, "用法: go run ./cmd/claw [-dir 工作目录] [-session 会话ID] [-prompt 首条任务]")
		fmt.Fprintln(output, "启动模式由 .env 的 mode 决定：cli（默认）或 feishu。")
		fmt.Fprintln(output, "CLI 启动后可直接输入消息，使用 /exit 或 /quit 退出。")
	}
	flags.StringVar(&options.InitialPrompt, "prompt", "", "可选的首条任务，执行后继续多轮对话")
	flags.StringVar(&options.WorkDir, "dir", ".", "Agent 运行的工作区目录路径")
	flags.StringVar(&options.SessionID, "session", "cli_default_session", "指定当前进程中的会话 ID")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errors.New("启动参数无效，请使用 -h 查看用法")
	}
	return app.Run(context.Background(), options, input, output)
}
