# go-tiny-claw

用 Go 编写的微型 AI 编程智能体，实现 ReAct（Reasoning + Acting）循环：模型每一轮自行决定输出文本还是调用工具，程序执行工具并把结果回填，直到模型不再调用工具为止。

## 特性

- **多轮对话**：终端内连续提问，同一会话共享上下文
- **多模型适配**：`LLMProvider` 接口，支持 OpenAI 兼容接口（含 DeepSeek 推理内容）和 Claude
- **内置工具**：`read_file`、`write_file`、`edit_file`（精确替换）、`bash`（前台 + 后台任务）、`spawn_subagent`、`skill`
- **并发安全**：工具并发执行，同路径串行、跨路径并行，`bash` 与文件操作互斥
- **自愈机制**：错误恢复提示、连续失败死循环检测、上下文压缩
- **扩展**：`.claw/skills/` 按需加载技能；`.claw/mcp.json` 配置 MCP 服务（当前默认未启用，见 `internal/app/tools.go`）
- **可观测**：`.claw/traces/` 下的 span 链路记录（含模型输入输出）、会话 Token 与费用统计
- **飞书模式**：通过飞书机器人下发任务，危险命令需卡片审批
- **评测**：`cmd/bench` 基准评测

架构图见 [docs/architecture.md](docs/architecture.md)。

## 快速开始

需要 Go 1.25+。在项目根目录创建 `.env`：

```env
API_KEY=你的密钥
baseURL=模型接口地址
# 可选：cli（默认）或 feishu
mode=cli
# 飞书模式需要
FEISHU_APP_ID=
FEISHU_APP_SECRET=
```

运行：

```bash
go run ./cmd/claw/
```

可选参数：`-dir` 工作目录，`-session` 会话 ID，`-prompt` 首条任务。启动后输入任务并回车，`/exit` 或 `/quit` 退出。

```bash
# 测试
go test ./...

# 基准评测
go run ./cmd/bench/
```

## 目录结构

| 目录 | 说明 |
| --- | --- |
| `cmd/claw` | 程序入口 |
| `cmd/bench` | 基准评测入口 |
| `internal/app` | 配置、依赖装配、模式选择、资源生命周期 |
| `internal/cli` | 终端多轮对话 |
| `internal/engine` | ReAct 主循环、子智能体、Reporter |
| `internal/session` | 会话历史与用量累计 |
| `internal/prompt` | 系统提示词组装 |
| `internal/memory` | 上下文压缩 |
| `internal/recovery` | 工具错误恢复提示 |
| `internal/provider` | LLM 适配层 |
| `internal/tools` | 工具与注册表（锁、中间件、并发） |
| `internal/background` | 后台任务管理 |
| `internal/skill` | 技能加载 |
| `internal/mcpclient` | MCP stdio 客户端 |
| `internal/feishu` | 飞书机器人与审批 |
| `internal/trace` | 链路追踪 |
| `internal/usage` | 用量与费用统计 |
| `internal/eval` | 评测框架 |
| `internal/schema` | 共享领域类型 |

更详细的模块说明见 [CLAUDE.md](CLAUDE.md)。
