# internal/tools 目录指南

## 整体职责
本目录实现工具注册、锁调度和具体工具执行，是 agent 与文件系统/shell 交互的唯一入口。

## 文件速览
- `registry.go` — 工具注册中心 + 并发调度器。所有工具调用必须经过 `ExecuteParallel`，禁止绕过 Registry 直接调 `tool.Execute()`
- `lockmgr.go` — 双层锁：全局 RWMutex（bash 与文件工具互斥）+ 按路径 RWMutex 池（同路径串行，跨路径并行）
- `bash.go` — shell 执行，30s 超时，30KB 截断。**不实现 LockHinter**，因此执行时会抢占全局写锁
- `read_file.go` — 读取文件，支持行范围，50KB 截断。实现 LockHinter（读锁）
- `write_file.go` — 创建/覆盖文件，自动创建父目录。实现 LockHinter（写锁）
- `edit_file.go` — 精确字符串替换，两阶段算法。实现 LockHinter（写锁）。公共函数 `Replace()` 供测试用
- `subagent.go` — 生成只读子智能体。通过 `AgentRunner` 接口打破循环依赖

## 关键约定
- 新工具必须实现 `BaseTool` 接口的三个方法
- 涉及文件操作的工具**必须**实现 `LockHinter`，否则会和 bash 产生竞态
- 工具的错误应返回 `*schema.ToolError` 带 `ErrorCode`，以便 RecoveryManager 匹配
- `ExecuteParallel` 的信号量上限是 5，不要同时发起超过 5 个同轮工具调用
