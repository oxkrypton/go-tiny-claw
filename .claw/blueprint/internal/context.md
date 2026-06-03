# internal/context 目录指南

## 整体职责
本目录负责在每次 ReAct 迭代中动态组装系统提示词，并提供上下文安全防护（压缩、错误恢复）。所有模块都是无状态的，每次 Build() 都从磁盘和环境重新读取。

## 文件速览
- `composer.go` — PromptComposer：系统提示词组装入口。注入顺序：核心身份 → plan mode 指令（可选）→ AGENTS.md → harness → skills。Build() 每轮循环都会被调用
- `compactor.go` — Compactor：上下文 OOM 防护。水位线 20,000 字符，保留最近 6 条消息。远期工具结果全量遮蔽，近期长内容掐头去尾（各保留 500 字符）。不修改 System 消息和 ToolCalls 字段
- `recover.go` — RecoveryManager：工具失败时根据 ErrorCode 注入中文恢复建议。优先 code 精确匹配，code 为空时回退到字符串模式匹配
- `skill.go` — SkillLoader：扫描 `.claw/skills/*/SKILL.md`，解析 YAML frontmatter，格式化为系统提示词技能块
- `harness.go` — HarnessLoader：扫描 `.claw/harness/*.md`，注入目录级架构导航

## 关键约定
- 所有 Loader 返回空字符串时静默跳过，不影响 Build() 结果
- Compactor 的 MaxChars 和 RetainLastMsgs 是硬编码的，如需调整在 `NewAgentEngine` 中修改
- RecoveryManager 的提示是中文的，如果换模型语言需同步修改
- 新增 Loader 的模式：在 PromptComposer 中加字段 → NewPromptComposer 中初始化 → Build() 中按顺序注入
- harness 应放在 skill 之前注入，因为架构导航比技能更基础
