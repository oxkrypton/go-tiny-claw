// cmd/claw/main.go
package main

import (
	"context"
	"log"
	"os"

	"github.com/oxkrypton/go-tiny-claw/internal/engine"
	"github.com/oxkrypton/go-tiny-claw/internal/provider"
	"github.com/oxkrypton/go-tiny-claw/internal/schema"
	"github.com/oxkrypton/go-tiny-claw/internal/tools"
)

func main() {
	//获取工作区
	workDir, _ := os.Getwd()
	workDir+="/testdata"

	// 1. 初始化真实的 Provider大脑
	// 这里你可以任意切换 NewClaudeProvider 或 NewOpenAIProvider，效果完全一致！
	llmProvider := provider.NewOpenAIProvider("deepseek-v4-flash")

	// 2. 注入伪造的工具注册表
	registry := tools.NewRegistry()

	// 3.挂载 4 大基础工具
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	// 4. 实例化并运行引擎
	eng := engine.NewAgentEngine(llmProvider, registry, false)

	//便于测试的终端输出器
	reporter := engine.NewTerminalReporter()

	sessionID := "test_oom_protection_001"
	sess := engine.GlobalSessionMgr.GetOrCreate(sessionID, workDir)

	// 通过命令行参数接收用户的 prompt
	prompt := `
我当前目录下有一个 auth.go 文件。
请修改 auth.go 中的 login 函数。 
请直接使用 edit_file 工具替换下面的代码块，将判断条件改为同时允许"admin"、"root"和"guest"三种用户登录： 
// 鉴权入口函数 
func login(user string) bool {
	// 检查用户名 
	if user == "admin" {
		return true 
	} 
	return false 
}
	`

	log.Printf("\n>>> 🚀 收到指令: %s\n", prompt)
	
	sess.Append(schema.Message{Role: schema.RoleUser, Content: prompt})

	err := eng.Run(context.Background(), sess, reporter)
	if err != nil {
		log.Fatal("fail to start engine: %v", err)
	}

	// 5. 通过 WebSocket 长连接启动飞书 Bot（阻塞）
	// log.Println("go-tiny-claw 飞书长连接模式启动中...")
	// if err := feishu.NewFeishuBot(eng).Start(context.Background()); err != nil {
	// 	log.Fatalf("飞书长连接启动失败: %v", err)
	// }
}
