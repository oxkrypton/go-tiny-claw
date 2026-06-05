package engine

import (
	"context"
	"fmt"
	"log"

	ctxpkg "github.com/oxkrypton/go-tiny-claw/internal/context"
	"github.com/oxkrypton/go-tiny-claw/internal/observability"
	"github.com/oxkrypton/go-tiny-claw/internal/provider"
	"github.com/oxkrypton/go-tiny-claw/internal/schema"
	"github.com/oxkrypton/go-tiny-claw/internal/tools"
)

// AgentEngine 是微型 OS 的核心驱动
type AgentEngine struct {
	provider  provider.LLMProvider
	registry  tools.Registry
	PlanMode  bool                    // 暴露给外部的计划模式开关
	compactor *ctxpkg.Compactor       // 压缩器实例
	recovery  *ctxpkg.RecoveryManager // 错误增强
	injector  *ReminderInjector       //提醒注入器
}

func NewAgentEngine(p provider.LLMProvider, r tools.Registry, planMode bool) *AgentEngine {
	return &AgentEngine{
		provider:  p,
		registry:  r,
		PlanMode:  planMode,
		compactor: ctxpkg.NewCompactor(20000, 6),
		recovery:  ctxpkg.NewRecoveryManager(),
		injector:  NewReminderInjector(),
	}
}

// Run 启动 Agent 的生命周期
func (e *AgentEngine) Run(ctx context.Context, session *ctxpkg.Session, reporter Reporter) error {
	log.Printf("[Engine] 会话 [%s]，锁定工作区: %s (PlanMode: %v)\n", session.ID, session.WorkDir, e.PlanMode)

	// 开启 Root Span，记录整个任务的生命周期
	ctx, rootSpan := observability.StartSpan(ctx, "Agent.Run")
	rootSpan.AddAttribute("SessionID", session.ID)
	rootSpan.AddAttribute("WorkDir", session.WorkDir)

	// defer 保证在引擎退出时，无论成功失败，都能结束根 Span 并导出 Trace 报告
	defer func() {
		rootSpan.EndSpan()
		_ = observability.ExportTraceToFile(rootSpan, session.WorkDir, session.ID)
		log.Printf("📊 [Tracing] 本次任务的执行回放链路已保存至工作区的 .claw/traces 目录下\n")
	}()

	// 根据当前 Session 的工作区，动态组装最新的 System Prompt
	composer := ctxpkg.NewPromptComposer(session.WorkDir, e.PlanMode)
	systemMsg := composer.Build()

	turnCount := 0

	// 2. The Main Loop: 心跳开始 (标准的 ReAct 循环)
	for {
		turnCount++

		//记录单次 Turn 循环
		turnCtx, turnSpan := observability.StartSpan(ctx, fmt.Sprintf("Turn-%d", turnCount))

		// 获取当前挂载的所有工具定义
		availableTools := e.registry.GetAvailableTools()

		// 1.【上下文组装】: System Prompt + 截取最近的 20 条消息作为 Working Memory
		workingMemory := session.GetWorkingMemory(20)

		var contextHistory []schema.Message
		contextHistory = append(contextHistory, systemMsg)
		contextHistory = append(contextHistory, workingMemory...)
		compactedContext := e.compactor.Compact(contextHistory)

		// 记录发给模型的实际上下文大小，有助于排查幻觉
		turnSpan.AddAttribute("context_message_count", len(compactedContext))

		// ================= Action =================
		//记录 Action 调用
		actCtx, actSpan := observability.StartSpan(turnCtx, "LLM.Action")
		// 每一轮都直接注入工具，让模型在同一次响应中决定回复文本或发起工具调用。
		actionResp, err := e.provider.Generate(actCtx, compactedContext, availableTools)
		actSpan.EndSpan() //结束行动跨度
		if err != nil {
			return fmt.Errorf("Action 阶段生成失败: %w", err)
		}

		// 将大模型的行动响应持久化到 Session 和 contextHistory 中
		session.Append(*actionResp)
		contextHistory = append(contextHistory, *actionResp)

		//如果模型回复了纯文本，打印出来 (通常是思考过程或最终结果)
		if actionResp.Content != "" && reporter != nil {
			// 【触发 Reporter】: 输出阶段性总结或最终回复
			reporter.OnMessage(ctx, actionResp.Content)
		}

		// 3. 退出条件判断
		// 如果模型没有请求任何工具调用，说明它认为任务已经完成，跳出循环
		if len(actionResp.ToolCalls) == 0 {
			// 结束本轮 Turn 的 Span
			turnSpan.EndSpan()
			break
		}

		// 4. 执行行动 (Action) 与 获取观察结果 (Observation)
		// 并行调度与路径锁的细节都收敛在 registry.ExecuteParallel 内：
		// - 同路径串行（写独占、读共享），跨路径并行
		// - bash 等无法静态分析路径的工具会拿全局写锁，期间挡住所有文件类工具
		rawResults := e.registry.ExecuteParallel(turnCtx, actionResp.ToolCalls, reporter)

		// 5. 后处理增强 (Error Recovery + Reminder Injection)
		// 必须在所有工具结果 append 完之后才能追加 reminder，否则会破坏工具调用协议的消息顺序。
		observationMsgs := make([]schema.Message, len(rawResults))
		var reminderMsg *schema.Message

		for i, result := range rawResults {
			call := actionResp.ToolCalls[i]

			// 错误增强：用 RecoveryManager 改写 Output，注入可操作的恢复建议
			if result.IsError {
				result.Output = e.recovery.AnalyzeAndInject(call.Name, result.Output, result.ErrorCode)
				log.Printf(" -> [Go-%d] ❌ %s: %s\n", i, call.Name, result.Output)
			} else {
				log.Printf(" -> [Go-%d] ✅ %s (返回 %d 字节)\n", i, call.Name, len(result.Output))
			}

			// 死循环探测：只收集，不立刻写入 Session
			if nudge := e.injector.CheckAndInject(call, result); nudge != nil && reminderMsg == nil {
				reminderMsg = nudge
			}

			observationMsgs[i] = schema.Message{
				Role:       schema.RoleUser,
				Content:    result.Output,
				ToolCallID: call.ID,
			}
			contextHistory = append(contextHistory, observationMsgs[i])
		}

		// 持久化：先写入全部工具结果，保持协议要求的顺序
		session.Append(observationMsgs...)

		// 再追加 reminder（最多一条），放在最末尾以获得最高的近因效应权重
		if reminderMsg != nil {
			session.Append(*reminderMsg)
			contextHistory = append(contextHistory, *reminderMsg)
		}

		// 结束本轮 Turn 的 Span
		turnSpan.EndSpan()
		// 循环回到开头，模型将带着这一批新的 Observation 继续它的下一轮思考...
	}

	return nil
}

// RunSub 是专为 Subagent 拉起的一次性受限循环。不依赖外部 Session，执行完就结束。
func (e *AgentEngine) RunSub(ctx context.Context, taskPrompt string, readOnlyRegistry tools.Registry, reporter any) (string, error) {
	// 可视化：将透传进来的 reporter 断言为 Reporter 接口，
	// 后续打上 [Subagent] 标记让终端用户看到 Subagent 正在干嘛
	var r Reporter
	if reporter != nil {
		var ok bool
		r, ok = reporter.(Reporter)
		if !ok {
			log.Printf("[Subagent] 警告：reporter 类型断言失败，跳过可视化输出")
		}
	}

	contextHistory := []schema.Message{
		{
			Role: schema.RoleSystem,
			Content: `你是一个专门负责深度探索的探路者 (Explorer Subagent)。
你的任务是根据主架构师的指令，在当前工作区内仔细阅读代码、查阅日志，搜集足够的信息。

【核心纪律】
1. 你必须、且只能依靠内置工具（如 bash 的 find/grep，或 read_file）去寻找答案。绝对不允许凭空捏造或猜测！
2. 如果你没有找到确切的答案，你必须继续使用工具深入搜索。
3. 当且仅当你找到了确切的线索后，停止调用工具，直接输出一段纯文本作为你的终极汇报。主Agent会根据你的汇报来做下一步决策。`,
		},
		{
			Role:    schema.RoleUser,
			Content: taskPrompt,
		},
	}

	const maxSubTurns = 10
	turnCount := 0

	for {
		turnCount++
		if turnCount > maxSubTurns {
			return "", fmt.Errorf("Subagent探索过于深入，超过 %d 轮被强制召回，请主 Agent 给它更明确的指令", maxSubTurns)
		}

		// Subagent 仅能获取传入的只读工具注册表
		availableTools := readOnlyRegistry.GetAvailableTools()

		actionResp, err := e.provider.Generate(ctx, contextHistory, availableTools)
		if err != nil {
			return "", fmt.Errorf("Subagent推理失败: %w", err)
		}

		contextHistory = append(contextHistory, *actionResp)

		// 可视化：模型输出了纯文本时通知外部（如终端、飞书）
		if actionResp.Content != "" && r != nil {
			r.OnMessage(ctx, actionResp.Content)
		}

		// 退出条件：Subagent 一旦不调用工具了，说明它做好了总结汇报
		if len(actionResp.ToolCalls) == 0 {
			return actionResp.Content, nil
		}

		// 可视化：逐条通知外部 Subagent 正在调用什么工具，打上 [Subagent] 前缀与主 Agent 区分
		for _, call := range actionResp.ToolCalls {
			if r != nil {
				r.OnToolCall(ctx, fmt.Sprintf("[Subagent] %s", call.Name), string(call.Arguments))
			}
		}

		// 使用只读注册表执行工具 —— 确保 Subagent 只能读，不能写/删/执行危险命令
		rawResults := readOnlyRegistry.ExecuteParallel(ctx, actionResp.ToolCalls, nil)

		for i, result := range rawResults {
			call := actionResp.ToolCalls[i]

			// 可视化：通知工具执行结果
			if r != nil {
				output := result.Output
				if len(output) > 200 {
					output = output[:200] + "...(已截断)"
				}
				r.OnToolResult(ctx, fmt.Sprintf("[Subagent] %s", call.Name), output, result.IsError)
			}

			if result.IsError {
				log.Printf(" -> [Sub-%d] ❌ %s: %s\n", i, call.Name, result.Output)
			} else {
				log.Printf(" -> [Sub-%d] ✅ %s (返回 %d 字节)\n", i, call.Name, len(result.Output))
			}

			contextHistory = append(contextHistory, schema.Message{
				Role:       schema.RoleUser,
				Content:    result.Output,
				ToolCallID: call.ID,
			})
		}
	}
}
