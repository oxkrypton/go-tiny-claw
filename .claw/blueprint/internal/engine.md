# internal/engine 目录指南

## 整体职责
本目录实现 agent 的核心生命周期：ReAct 循环、会话管理、外部输出和死循环检测。是连接 provider、tools、context 三层的中枢。

## 文件速览
- `loop.go` — AgentEngine：ReAct 主循环 + RunSub（子智能体）。每轮构建上下文 → 调 LLM → 并发执行工具 → 后处理 → 追加结果 → 判断退出。PlanMode 开关在此控制。每次迭代 dump `session.json`
- `session.go` — Session：线程安全的消息历史。`GetWorkingMemory(limit)` 截取最近 N 条，含孤儿 ToolResult 跳过逻辑。SessionManager + GlobalSessionMgr 管理多会话
- `reporter.go` — Reporter 接口：OnThinking / OnToolCall / OnToolResult / OnMessage 四个回调，供 CLI、飞书等不同输出端实现
- `terminal_reporter.go` — TerminalReporter：CLI 输出，带 emoji 和层级缩进（子智能体前缀 `[Subagent]`）
- `reminder.go` — ReminderInjector：对连续失败的工具调用做 MD5 指纹，同一指纹失败 3 次后注入 RoleUser 干预消息打破死循环

## 关键约定
- `AgentEngine.Run()` 是唯一对外入口，不要在外部直接操作 session 或调用 provider
- `RunSub()` 的子智能体限制 10 轮 + 只读注册表，不可突破
- 向 session 追加消息必须用 `Append()`，它持有写锁
- `GetWorkingMemory` 的 limit 当前硬编码为 20，改之前确认模型上下文窗口够用
- session.json 写在工作目录（`os.Getwd()`），不是 testdata。多会话同时跑会互相覆盖
