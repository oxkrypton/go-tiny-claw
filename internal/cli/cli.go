// Package cli 提供终端多轮对话，不负责服务初始化或工具装配。
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/oxkrypton/go-tiny-claw/internal/engine"
	"github.com/oxkrypton/go-tiny-claw/internal/schema"
	sessionpkg "github.com/oxkrypton/go-tiny-claw/internal/session"
)

// Runner 是终端对话需要的任务执行能力。
type Runner interface {
	Run(context.Context, *sessionpkg.Session, engine.Reporter) error
}

// Conversation 持有一段终端对话所需的依赖。
type Conversation struct {
	runner   Runner
	session  *sessionpkg.Session
	reporter engine.Reporter
}

func New(runner Runner, sess *sessionpkg.Session, reporter engine.Reporter) *Conversation {
	return &Conversation{runner: runner, session: sess, reporter: reporter}
}

// Run 逐条处理输入；每轮复用同一份会话。
func (c *Conversation) Run(ctx context.Context, input io.Reader, output io.Writer, initialPrompt string) error {
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
			c.session.Append(schema.Message{Role: schema.RoleUser, Content: prompt})
			if err := c.runner.Run(ctx, c.session, c.reporter); err != nil {
				fmt.Fprintf(output, "\n❌ 本轮任务执行失败: %v\n", err)
			} else {
				fmt.Fprintf(output, "✨ 本轮完成，耗时: %v\n", time.Since(start))
			}
			fmt.Fprintf(output, "💰 会话累计消耗: $%.6f | Token: Input %d, Output %d\n\n",
				c.session.TotalCostCNY, c.session.TotalPromptTokens, c.session.TotalCompletionTokens)
		}

		if endOfInput {
			fmt.Fprintln(output, "\n👋 对话已结束。")
			return nil
		}
		prompt = ""
	}
}
