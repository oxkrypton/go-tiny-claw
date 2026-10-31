package app

import (
	"context"
	"fmt"

	"github.com/oxkrypton/go-tiny-claw/internal/background"
	"github.com/oxkrypton/go-tiny-claw/internal/engine"
	"github.com/oxkrypton/go-tiny-claw/internal/mcpclient"
	"github.com/oxkrypton/go-tiny-claw/internal/skill"
	"github.com/oxkrypton/go-tiny-claw/internal/tools"
)

// 保留当前关闭 MCP 工具注册的状态，避免结构调整后启动外部服务。
const enableMCPTools = false

type toolsets struct {
	main     tools.Registry
	subagent tools.Registry
}

func buildToolsets(ctx context.Context, workDir string, bgManager *background.TaskManager, mcp *mcpclient.Manager) (toolsets, error) {
	main := tools.NewRegistry()
	main.Register(tools.NewReadFileTool(workDir))
	main.Register(tools.NewWriteFileTool(workDir))
	main.Register(tools.NewEditFileTool(workDir))
	main.Register(tools.NewBashTool(workDir, bgManager))
	skill.Register(main, workDir)

	subagent := tools.NewRegistry()
	subagent.Register(tools.NewReadFileTool(workDir))
	subagent.Register(tools.NewBashTool(workDir, bgManager))

	if enableMCPTools {
		if err := mcp.Register(ctx, main); err != nil {
			return toolsets{}, fmt.Errorf("注册 MCP 工具失败: %w", err)
		}
	}
	return toolsets{main: main, subagent: subagent}, nil
}

// 引擎创建完成后再注册，工具装配本身不负责创建引擎或输出器。
func (t toolsets) registerSubagent(runner tools.AgentRunner, reporter engine.Reporter) {
	t.main.Register(tools.NewSubagentTool(runner, t.subagent, reporter))
}
