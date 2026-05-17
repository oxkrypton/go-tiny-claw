// cmd/claw/main.go
package main

import (
	"context"
	"log"
	"os"

	"github.com/oxkrypton/go-tiny-claw/internal/engine"
	"github.com/oxkrypton/go-tiny-claw/internal/provider"
	"github.com/oxkrypton/go-tiny-claw/internal/tools"
)

func main() {
	//获取工作区
	workDir, _ := os.Getwd()

	// 1. 初始化真实的 Provider大脑
	// 这里你可以任意切换 NewClaudeProvider 或 NewOpenAIProvider，效果完全一致！
	llmProvider := provider.NewOpenAIProvider("deepseek-v4-flash")

	// 2. 注入伪造的工具注册表
	registry := tools.NewRegistry()

	// 3.初始化真实的 Tool Registry
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	// 4. 实例化并运行引擎，开启 EnableThinking = true (开启慢思考阶段)
	eng := engine.NewAgentEngine(llmProvider, registry, workDir, false)

	// 设定测试任务：在同一轮内**密集冲突同一路径**，让锁日志显示出真实的等待。
	//
	// 关键设计：
	//   - 6 个 edit 都打在同一个文件 testdata/sandbox/code/server.go 上，
	//     必然在 path 锁上排成一队，第 2 个之后每一个都会先 WAIT 再 GOT。
	//   - 同时穿插 2 个对 server.go 的 read，写者持锁期间它们也得等。
	//   - 再发一个 bash，演示 bash 抢全局写锁会把所有文件类工具挡住。
	prompt := `
请在**同一次回复中并行**发出下面所有工具调用，不要拆成多轮。
注意：多个 edit_file 故意都改 testdata/sandbox/code/server.go 这一个文件，
就是想观察同路径写写在 PathLockManager 中如何被串行化。

read 调用：
1. read_file 读 testdata/sandbox/code/server.go
2. read_file 读 testdata/sandbox/code/server.go（再读一次）

edit 调用（全部针对 testdata/sandbox/code/server.go）：
3. edit_file 把 "listening" 改为 "Listening"
4. edit_file 把 "Listening" 改为 "LISTENING"
5. edit_file 把 ":%d" 改为 "port=%d"
6. edit_file 把 "port int" 改为 "port uint16"

bash 调用：
7. bash 执行 wc -l testdata/sandbox/code/server.go

完成后简单总结一下 server.go 最终被改成了什么样子。
注意：edit_file 的 old_text 必须和上一步的实际文件内容匹配——
但请你**先并行发出全部 7 个调用**，因为我要观察并发情况下的锁等待行为，
即使后几个 edit 因为 old_text 不匹配而失败也没关系，那本身就是测试的一部分。
`

	err := eng.Run(context.Background(), prompt)
	if err != nil {
		log.Fatalf("引擎运行崩溃: %v", err)
	}
}
