// 工具的并发执行器
package tools

import (
	"context"

	"github.com/oxkrypton/go-tiny-claw/internal/schema"
	"golang.org/x/sync/errgroup"
)

// ExecuteParallel 并发执行所有 tool call，保证结果顺序与 toolcalls 一致
func ExecuteParallel(ctx context.Context, reg Registry, toolcalls []schema.ToolCall, maxConcurrency int) []schema.ToolResult {
	//工具的最大并发数
	if maxConcurrency <= 0 {
		maxConcurrency = 10
	}

	// 预分配结果切片，长度等于工具调用数
	results := make([]schema.ToolResult, len(toolcalls))

	var g errgroup.Group
	g.SetLimit(maxConcurrency)

	for i, call := range toolcalls {
		i, call := i, call // 闭包陷阱，必须拷贝
		g.Go(func() error {
			//执行单个tool
			result := reg.Execute(ctx, call)
			//将toolresult按顺序写入对应下标
			results[i] = result

			// 工具报错不中断其他 goroutine，错误走 ToolResult.IsError上报
			return nil
		})
	}

	// 阻塞等待所有 goroutine 完成
	g.Wait()
	return results
}
