package engine

import (
	"context"
	"fmt"
	"strings"
)

// TerminalReporter 实现了 Reporter 接口，用于在终端直观地打印 Agent 的状态
type TerminalReporter struct{}

func NewTerminalReporter() *TerminalReporter {
	return &TerminalReporter{}
}

func (r *TerminalReporter) OnThinking(ctx context.Context) {
	fmt.Printf("\n[🤔 思考中] 模型正在推理...\n")
}

func (r *TerminalReporter) OnToolCall(ctx context.Context, toolName string, args string) {
	if strings.HasPrefix(toolName, "[Subagent]") {
		// Subagent 工具调用缩进显示，与主 Agent 形成层次
		name := strings.TrimPrefix(toolName, "[Subagent] ")
		fmt.Printf("  🔍 探查 › %s\n", name)
		displayArgs := strings.ReplaceAll(args, "\n", "\\n")
		if len(displayArgs) > 120 {
			displayArgs = displayArgs[:120] + "..."
		}
		fmt.Printf("     %s\n", displayArgs)
		return
	}
	fmt.Printf("[🛠️ 调用工具] %s\n", toolName)
	displayArgs := strings.ReplaceAll(args, "\n", "\\n")
	displayArgs = strings.ReplaceAll(displayArgs, "\r", "\\r")
	if len(displayArgs) > 150 {
		displayArgs = displayArgs[:150] + "... (已截断)"
	}
	fmt.Printf("   参数: %s\n", displayArgs)
}

func (r *TerminalReporter) OnToolResult(ctx context.Context, toolName string, result string, isError bool) {
	if strings.HasPrefix(toolName, "[Subagent]") {
		name := strings.TrimPrefix(toolName, "[Subagent] ")
		if isError {
			fmt.Printf("  ❌ 探查 › %s\n", name)
		} else {
			fmt.Printf("  ✅ 探查 › %s\n", name)
		}
		return
	}
	if isError {
		fmt.Printf("[❌ 执行失败] %s\n", toolName)
		if result != "" {
			fmt.Printf("   错误: %s\n", result)
		}
	} else {
		fmt.Printf("[✅ 执行成功] %s\n", toolName)
	}
}

func (r *TerminalReporter) OnMessage(ctx context.Context, content string) {
	if content == "" {
		return
	}
	fmt.Printf("\n🤖 Agent 回复:\n%s\n\n", content)
}
