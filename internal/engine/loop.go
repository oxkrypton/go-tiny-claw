package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

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
	//慢思考模式开关
	EnableThinking bool
}

func NewAgentEngine(p provider.LLMProvider, r tools.Registry, workDir string, enableThinking bool) *AgentEngine {
	return &AgentEngine{
		provider:       p,
		registry:       r,
		WorkDir:        workDir,
		EnableThinking: enableThinking,
	}
}

// Run 启动 Agent 的生命周期
func (e *AgentEngine) Run(ctx context.Context, userPrompt string) error {
	log.Printf("[Engine] 引擎启动，锁定工作区: %s\n", e.WorkDir)
	log.Printf("[Engine] 慢思考模式 (Thinking Phase): %v\n", e.EnableThinking)

	// 1. 初始化会话的 Context (上下文内存)
	// 在真实的场景中，这里会由动态 Prompt 组装器加载 AGENTS.md。目前先硬编码。
	contextHistory := []schema.Message{
		{
			Role: schema.RoleSystem,
			Content: `You are go-tiny-claw, an expert coding assistant operating in a workspace.
					工具使用规范（必须遵守）：
					1. 修改已有文件时，必须使用 edit_file 工具进行局部替换（提供 path、old_text、new_text），禁止使用 sed/awk/perl 等 bash 命令修改文件内容。
					2. 读取文件内容时，优先使用 read_file 工具（支持 start_line/end_line 分段读取），而非 cat/head/tail。
					3. bash 工具仅用于：执行程序、编译构建、查看系统状态、创建目录等非文件编辑操作。
					4. 创建新文件时使用 write_file 工具。
					5. 如果 read_file 返回截断提示，使用 start_line/end_line 参数分段读取，不要切换到 bash。`,
		},
		{
			Role:    schema.RoleUser,
			Content: userPrompt,
		},
	}

	turnCount := 0

	// 2. The Main Loop: 心跳开始 (标准的 ReAct 循环)
	for {
		turnCount++
		log.Printf("========== [Turn %d] 开始 ==========\n", turnCount)

		// 将当前轮次的完整 context 写入 session.json，方便调试
		e.dumpSession(turnCount, contextHistory)

		// 获取当前挂载的所有工具定义
		availableTools := e.registry.GetAvailableTools()

		// Phase 1: 慢思考阶段 (Thinking) - 剥夺工具，强制规划
		if e.EnableThinking {
			log.Println("[Engine][Phase 1] 剥夺工具访问权，强制进入慢思考与规划阶段...")
			// 核心机制：传入的 availableTools 为 nil！
			// 大模型看不到任何 JSON Schema，被迫只能输出纯文本的思考过程。
			thinkResp, err := e.provider.Generate(ctx, contextHistory, nil)
			if err != nil {
				return fmt.Errorf("Thinking 阶段生成失败: %w", err)
			}

			if thinkResp.Content != "" {
				fmt.Printf("🧠 [内部思考 Trace]: %s\n", thinkResp.Content)
				contextHistory = append(contextHistory, *thinkResp)
			}
		}

		// Phase 2: 行动阶段 (Action) - 恢复工具，顺着规划执行
		log.Println("[Engine][Phase 2] 恢复工具挂载，等待模型采取行动...")

		// contextHistory 包含了上一阶段的 Thinking Trace，模型会顺着逻辑结合恢复的 availableTools 发起调用
		actionResp, err := e.provider.Generate(ctx, contextHistory, availableTools)
		if err != nil {
			return fmt.Errorf("Action 阶段生成失败: %w", err)
		}

		//将模型的响应完整追加到上下文历史中
		contextHistory = append(contextHistory, *actionResp)

		//如果模型回复了纯文本，打印出来 (通常是思考过程或最终结果)
		if actionResp.Content != "" {
			fmt.Printf("🤖 模型对外回复: %s \n", actionResp.Content)
		}

		// 3. 退出条件判断
		// 如果模型没有请求任何工具调用，说明它认为任务已经完成，跳出循环
		if len(actionResp.ToolCalls) == 0 {
			log.Println("[Engine] 模型未请求调用工具, 任务完成，退出循环")
			e.dumpSession(turnCount, contextHistory) // 最终状态也写入
			break
		}

		// 4. 执行行动 (Action) 与 获取观察结果 (Observation)
		log.Printf("[Engine] 模型请求调用 %d 个工具...\n", len(actionResp.ToolCalls))

		// 预分配一个固定长度的切片，用于安全地存放各个并发工具的执行结果（Observation） /
		// 长度与 ToolCalls 的数量完全一致
		observationMsgs := make([]schema.Message, len(actionResp.ToolCalls))

		//声明 WaitGroup 用于阻塞等待所有携程完成
		var wg sync.WaitGroup

		for i, toolCall := range actionResp.ToolCalls {
			wg.Add(1) //增加计数器

			// 开启协程, 需要将索引 i 和 toolCall 作为参数传入匿名函数, 防止闭包变量
			go func(idx int, call schema.ToolCall) {
				defer wg.Done() //协程结束时计数器减一

				log.Printf(" -> [Go-%d] 🛠️ 触发并行执行: %s\n", idx, call.Name)

				//调用低沉 Registry 执行工具
				result := e.registry.Execute(ctx, call)

				if result.IsError {
					log.Printf(" -> [Go-%d] ❌ 工具执行报错: %s\n", idx, result.Output)
				} else {
					log.Printf(" -> [Go-%d] ✅ 工具执行成功 (返回 %d 字节)\n", idx, len(result.Output))
				}

				// 将工具执行的观察结果 (Observation) 封装为 User Message 追加到上下文中
				// ToolCallID 必须携带！是维系大模型推理链条的关键
				obsMsg := schema.Message{
					Role:       schema.RoleUser,
					Content:    result.Output,
					ToolCallID: call.ID,
				}

				// 【线程安全】: 由于每个 Goroutine 操作的是预分配切片的不同索引，
				// 这里不需要加锁 (Mutex)，性能极高！
				observationMsgs[idx] = obsMsg

			}(i, toolCall)
		}

		// Join 阻塞等待: 主循环挂起, 直到所有的并发协程全部执行完毕
		wg.Wait()
		log.Println("[Engine] 所有并发工具执行完毕，开始聚合观察结果 (Observation)...")

		// 5. 聚合装填：将并行的结果，按照原本的顺序，一次性追加到上下文时间线中 /
		// 这等价于 contextHistory = append(contextHistory, observationMsgs...)
		for _, obs := range observationMsgs {
			contextHistory = append(contextHistory, obs)
		}
		// 循环回到开头，模型将带着这一批新的 Observation 继续它的下一轮思考...
	}
	
	return nil
}

// dumpSession 将当前轮次的完整上下文写入 session.json
func (e *AgentEngine) dumpSession(turn int, history []schema.Message) {
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

	sessionPath := filepath.Join(e.WorkDir, "testdata", "session.json")
	// 确保 testdata 目录存在
	os.MkdirAll(filepath.Dir(sessionPath), 0755)
	if err := os.WriteFile(sessionPath, jsonBytes, 0644); err != nil {
		log.Printf("[Session] 写入 session.json 失败: %v", err)
		return
	}

	log.Printf("[Session] Turn %d 的完整 context 已写入 session.json", turn)
}
