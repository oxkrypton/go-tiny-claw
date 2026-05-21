package engine

import (
	"context"
	"fmt"
	"log"

	ctxpkd "github.com/oxkrypton/go-tiny-claw/internal/context"
	"github.com/oxkrypton/go-tiny-claw/internal/provider"
	"github.com/oxkrypton/go-tiny-claw/internal/schema"
	"github.com/oxkrypton/go-tiny-claw/internal/tools"
)

// AgentEngine 是微型 OS 的核心驱动
type AgentEngine struct {
	provider provider.LLMProvider
	registry tools.Registry
	//WorkDir (工作区): 借鉴 OpenClaw的理念，Agent 必须有一个明确的物理边界
	WorkDir string
	// 动态加载sysprompt/skill
	composer ctxpkd.PromptComposer
}

func NewAgentEngine(p provider.LLMProvider, r tools.Registry, workDir string) *AgentEngine {
	return &AgentEngine{
		provider: p,
		registry: r,
		WorkDir:  workDir,
		composer: *ctxpkd.NewPromptComposer(workDir),
	}
}

// Run 启动 Agent 的生命周期
func (e *AgentEngine) Run(ctx context.Context, userPrompt string, reporter Reporter) error {
	log.Printf("[Engine] 引擎启动，锁定工作区: %s\n", e.WorkDir)

	// 1. 初始化会话的 Context (上下文内存), 动态 Prompt 组装器加载 AGENTS.md
	systemMsg := e.composer.Build()

	contextHistory := []schema.Message{
		systemMsg, // 注入动态组装的内核、AGENTS.md 与 Skills
		{Role: schema.RoleUser, Content: userPrompt},
	}

	turnCount := 0

	// 2. The Main Loop: 心跳开始 (标准的 ReAct 循环)
	for {
		turnCount++

		// 将当前轮次的完整 context 写入 session.json，方便调试
		//e.dumpSession(turnCount, contextHistory)

		// 获取当前挂载的所有工具定义
		availableTools := e.registry.GetAvailableTools()

		// ================= Action =================
		// 每一轮都直接注入工具，让模型在同一次响应中决定回复文本或发起工具调用。
		actionResp, err := e.provider.Generate(ctx, contextHistory, availableTools)
		if err != nil {
			return fmt.Errorf("Action 阶段生成失败: %w", err)
		}

		//将模型的响应完整追加到上下文历史中
		contextHistory = append(contextHistory, *actionResp)

		//如果模型回复了纯文本，打印出来 (通常是思考过程或最终结果)
		if actionResp.Content != "" && reporter != nil {
			// 【触发 Reporter】: 输出阶段性总结或最终回复
			reporter.OnMessage(ctx, actionResp.Content)
		}

		// 3. 退出条件判断
		// 如果模型没有请求任何工具调用，说明它认为任务已经完成，跳出循环
		if len(actionResp.ToolCalls) == 0 {
			// 将当前轮次的完整 context 写入 session.json，方便调试
			//e.dumpSession(turnCount, contextHistory)

			break
		}

		// 4. 执行行动 (Action) 与 获取观察结果 (Observation)
		// 并行调度与路径锁的细节都收敛在 registry.ExecuteParallel 内：
		//   - 同路径串行（写独占、读共享），跨路径并行
		//   - bash 等无法静态分析路径的工具会拿全局写锁，期间挡住所有文件类工具
		// engine 这里只负责把结果按原顺序拼回 contextHistory。

		results := e.registry.ExecuteParallel(ctx, actionResp.ToolCalls, reporter)

		for i, result := range results {
			call := actionResp.ToolCalls[i]
			if result.IsError {
				log.Printf(" -> [Go-%d] ❌ %s: %s\n", i, call.Name, result.Output)
			} else {
				log.Printf(" -> [Go-%d] ✅ %s (返回 %d 字节)\n", i, call.Name, len(result.Output))
			}
			// ToolCallID 必须携带，是维系大模型推理链条的关键
			contextHistory = append(contextHistory, schema.Message{
				Role:       schema.RoleUser,
				Content:    result.Output,
				ToolCallID: call.ID,
			})
		}
		// 循环回到开头，模型将带着这一批新的 Observation 继续它的下一轮思考...
	}

	return nil
}

// dumpSession 将当前轮次的完整上下文写入 session.json
// func (e *AgentEngine) dumpSession(turn int, history []schema.Message) {
// 	type sessionData struct {
// 		Turn    int              `json:"turn"`
// 		Context []schema.Message `json:"context"`
// 	}

// 	data := sessionData{
// 		Turn:    turn,
// 		Context: history,
// 	}

// 	jsonBytes, err := json.MarshalIndent(data, "", "  ")
// 	if err != nil {
// 		log.Printf("[Session] 序列化 context 失败: %v", err)
// 		return
// 	}

// 	sessionPath := filepath.Join(e.WorkDir, "testdata", "session.json")
// 	// 确保 testdata 目录存在
// 	os.MkdirAll(filepath.Dir(sessionPath), 0755)
// 	if err := os.WriteFile(sessionPath, jsonBytes, 0644); err != nil {
// 		log.Printf("[Session] 写入 session.json 失败: %v", err)
// 		return
// 	}

// 	log.Printf("[Session] Turn %d 的完整 context 已写入 session.json", turn)
// }
