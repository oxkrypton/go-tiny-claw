package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/oxkrypton/go-tiny-claw/internal/engine"
	"github.com/oxkrypton/go-tiny-claw/internal/schema"
	sessionpkg "github.com/oxkrypton/go-tiny-claw/internal/session"
	"github.com/oxkrypton/go-tiny-claw/internal/tools"
)

// 使用真实引擎检查每一轮传给模型的上下文，避免只验证输入循环。
type conversationProvider struct {
	calls     [][]schema.Message
	failFirst bool
}

func (p *conversationProvider) Generate(_ context.Context, messages []schema.Message, _ []schema.ToolDefinition) (*schema.Message, error) {
	p.calls = append(p.calls, append([]schema.Message(nil), messages...))
	if p.failFirst && len(p.calls) == 1 {
		return nil, errors.New("模拟服务暂时不可用")
	}
	return &schema.Message{Role: schema.RoleAssistant, Content: fmt.Sprintf("回复%d", len(p.calls))}, nil
}

func TestCLIConversation(t *testing.T) {
	longPrompt := strings.Repeat("测", 40000)
	tests := []struct {
		name          string
		input         string
		initialPrompt string
		wantPrompts   []string
	}{
		{name: "连续两轮对话", input: "我叫小明\n我叫什么名字？\n/exit\n", wantPrompts: []string{"我叫小明", "我叫什么名字？"}},
		{name: "忽略空白行", input: "\n  \n你好\n/quit\n后续内容\n", wantPrompts: []string{"你好"}},
		{name: "输入结束时处理最后一条消息", input: "没有换行的最后一条消息", wantPrompts: []string{"没有换行的最后一条消息"}},
		{name: "直接结束输入", input: ""},
		{name: "立即退出", input: "/exit\n不应执行\n"},
		{name: "兼容可选首条任务", initialPrompt: "第一条任务", input: "继续\n/exit\n", wantPrompts: []string{"第一条任务", "继续"}},
		{name: "支持长输入", input: longPrompt + "\n/exit\n", wantPrompts: []string{longPrompt}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &conversationProvider{}
			eng := engine.NewAgentEngine(p, tools.NewRegistry(), false)
			sess := sessionpkg.NewSession("cli-test", t.TempDir())
			var output bytes.Buffer
			if err := New(eng, sess, nil).Run(context.Background(), strings.NewReader(tt.input), &output, tt.initialPrompt); err != nil {
				t.Fatal(err)
			}
			if len(p.calls) != len(tt.wantPrompts) {
				t.Fatalf("期望执行 %d 轮，实际执行 %d 轮", len(tt.wantPrompts), len(p.calls))
			}
			for i, messages := range p.calls {
				// 每次请求都必须携带此前的用户消息和模型回复。
				if len(messages) != 2*i+2 || messages[0].Role != schema.RoleSystem {
					t.Fatalf("第 %d 轮上下文不完整，实际消息数 %d", i+1, len(messages))
				}
				for j := 0; j <= i; j++ {
					if msg := messages[2*j+1]; msg.Role != schema.RoleUser || msg.Content != tt.wantPrompts[j] {
						t.Fatalf("第 %d 轮丢失了第 %d 条用户消息", i+1, j+1)
					}
					if j < i {
						if msg := messages[2*j+2]; msg.Role != schema.RoleAssistant || msg.Content != fmt.Sprintf("回复%d", j+1) {
							t.Fatalf("第 %d 轮丢失了第 %d 条模型回复", i+1, j+1)
						}
					}
				}
			}
			if !strings.Contains(output.String(), "对话已结束") {
				t.Fatal("退出时缺少结束提示")
			}
		})
	}
}

func TestCLICanContinueAfterTaskFailure(t *testing.T) {
	p := &conversationProvider{failFirst: true}
	eng := engine.NewAgentEngine(p, tools.NewRegistry(), false)
	sess := sessionpkg.NewSession("cli-test", t.TempDir())
	var output bytes.Buffer
	if err := New(eng, sess, nil).Run(context.Background(), strings.NewReader("第一次\n重试\n/exit\n"), &output, ""); err != nil {
		t.Fatal(err)
	}
	if len(p.calls) != 2 || !strings.Contains(output.String(), "本轮任务执行失败") || !strings.Contains(output.String(), "本轮完成") {
		t.Fatalf("失败后未能继续对话：%s", output.String())
	}
}

type failedInput struct{}

func (failedInput) Read([]byte) (int, error) {
	return 0, errors.New("模拟输入故障")
}

func TestCLIInputFailure(t *testing.T) {
	// 输入失败前不应调用引擎。
	err := New(nil, nil, nil).Run(context.Background(), failedInput{}, io.Discard, "")
	if err == nil || err.Error() != "读取终端输入失败，请检查输入来源" {
		t.Fatalf("期望中文输入错误提示，实际为 %v", err)
	}
}
