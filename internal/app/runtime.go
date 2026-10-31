package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/oxkrypton/go-tiny-claw/internal/background"
	"github.com/oxkrypton/go-tiny-claw/internal/engine"
	"github.com/oxkrypton/go-tiny-claw/internal/mcpclient"
	"github.com/oxkrypton/go-tiny-claw/internal/provider"
	"github.com/oxkrypton/go-tiny-claw/internal/session"
	"github.com/oxkrypton/go-tiny-claw/internal/usage"
)

const modelName = "deepseek-v4-flash"

// runtime 仅供应用层装配使用，不传给 CLI 或飞书。
type runtime struct {
	engine     *engine.AgentEngine
	session    *session.Session
	background *background.TaskManager
	mcp        *mcpclient.Manager
	tools      toolsets
}

func newRuntime(ctx context.Context, cfg config, reporter engine.Reporter) (_ *runtime, err error) {
	rt := &runtime{session: session.NewSession(cfg.sessionID, cfg.workDir)}
	// 初始化中途失败也必须清理已经创建的资源。
	defer func() {
		if err != nil {
			err = errors.Join(err, rt.Close())
		}
	}()

	llm := provider.NewOpenAIProvider(modelName)
	tracked := usage.NewTracker(llm, modelName, rt.session)
	rt.background = background.NewTaskManager(cfg.workDir)
	rt.mcp, err = mcpclient.NewManager(cfg.workDir)
	if err != nil {
		return nil, fmt.Errorf("初始化 MCP 配置失败: %w", err)
	}

	sets, err := buildToolsets(ctx, cfg.workDir, rt.background, rt.mcp)
	if err != nil {
		return nil, err
	}
	rt.tools = sets
	rt.engine = engine.NewAgentEngine(tracked, sets.main, false)
	sets.registerSubagent(rt.engine, reporter)
	return rt, nil
}

// Close 按创建的相反顺序关闭资源，并允许重复调用。
func (r *runtime) Close() error {
	var err error
	if r.mcp != nil {
		if closeErr := r.mcp.Close(); closeErr != nil {
			err = fmt.Errorf("关闭 MCP 连接失败: %w", closeErr)
		}
		r.mcp = nil
	}
	if r.background != nil {
		r.background.Cleanup()
		r.background = nil
	}
	return err
}
