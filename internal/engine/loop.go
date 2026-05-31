package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	ctxpkg "github.com/oxkrypton/go-tiny-claw/internal/context"
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
func (e *AgentEngine) Run(ctx context.Context, session *Session, reporter Reporter) error {
	log.Printf("[Engine] 会话 [%s]，锁定工作区: %s (PlanMode: %v)\n", session.ID, session.WorkDir, e.PlanMode)

	turnCount := 0

	// 根据当前 Session 的工作区，动态组装最新的 System Prompt
	composer := ctxpkg.NewPromptComposer(session.WorkDir, e.PlanMode)
	systemMsg := composer.Build()

	// 2. The Main Loop: 心跳开始 (标准的 ReAct 循环)
	for {
		// 获取当前挂载的所有工具定义
		availableTools := e.registry.GetAvailableTools()

		// 1.【上下文组装】: System Prompt + 截取最近的 20 条消息作为 Working Memory
		workingMemory := session.GetWorkingMemory(20)

		var contextHistory []schema.Message
		contextHistory = append(contextHistory, systemMsg)
		contextHistory = append(contextHistory, workingMemory...)

		turnCount++

		// ================= Action =================
		// 每一轮都直接注入工具，让模型在同一次响应中决定回复文本或发起工具调用。
		actionResp, err := e.provider.Generate(ctx, contextHistory, availableTools)
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
			// 将当前轮次的完整 context 写入 session.json，方便调试
			e.dumpSession(turnCount, session, contextHistory)

			break
		}

		// 4. 执行行动 (Action) 与 获取观察结果 (Observation)
		// 并行调度与路径锁的细节都收敛在 registry.ExecuteParallel 内：
		// - 同路径串行（写独占、读共享），跨路径并行
		// - bash 等无法静态分析路径的工具会拿全局写锁，期间挡住所有文件类工具
		rawResults := e.registry.ExecuteParallel(ctx, actionResp.ToolCalls, reporter)

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

		// 将本轮完整 context（含 assistant(tool_calls) + tool_results + reminder）写入 session.json
		e.dumpSession(turnCount, session, contextHistory)
		// 循环回到开头，模型将带着这一批新的 Observation 继续它的下一轮思考...
	}

	return nil
}

// dumpSession 将当前轮次的完整上下文写入 session.json
func (e *AgentEngine) dumpSession(turn int, session *Session, history []schema.Message) {
	type sessionData struct {
		Turn    int              `json:"turn"`
		Context []schema.Message `json:"context"`
	}

	data := sessionData{
		Turn:    turn,
		Context: history,
	}

	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("[Session] 序列化 context 失败: %v", err)
		return
	}

	sessionPath := "session.json"
	if err := os.WriteFile(sessionPath, jsonBytes, 0644); err != nil {
		log.Printf("[Session] 写入 session.json 失败: %v", err)
		return
	}

	log.Printf("[Session] Turn %d 的完整 context 已写入 session.json", turn)
}
