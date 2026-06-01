# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 维护规则

**每次提交新功能后，必须检查本文件是否需要更新。** 新增的模块、工具、接口或架构变更都应及时反映在此文档中。提交前将"检查 CLAUDE.md 是否过期"作为必查项。

## Commands

```bash
# Build
go build ./...

# Run (requires .env with API_KEY and baseURL)
go run ./cmd/claw/

# Test (all packages)
go test ./...

# Test a single package
go test ./internal/tools/ -run TestName -v
```

## Architecture

**go-tiny-claw** is a miniature AI coding agent that implements the ReAct (Reasoning + Acting) loop. It talks to an LLM (Claude or OpenAI-compatible), executes tool calls, and feeds results back in a loop until the model decides it's done.

### Layer model (top → bottom)

1. **Entry point** (`cmd/claw/main.go`) — wires the provider, tool registry, and engine together. Creates a session via `GlobalSessionMgr`, registers all 5 tools (read_file, write_file, edit_file, bash, spawn_subagent) plus a read-only subagent registry, then calls `eng.Run()` with a user prompt.

2. **Engine** (`internal/engine/`) — the core ReAct loop in `loop.go`. Each turn:
   - Builds context from dynamic system prompt (via `PromptComposer`) + working memory (last 20 messages from session).
   - Calls the LLM with tools available — the model decides in a single call whether to emit text or tool calls.
   - Executes tool calls concurrently via `registry.ExecuteParallel()`.
   - Post-processes: error recovery hints (`RecoveryManager`), dead-loop detection (`ReminderInjector`), context compaction (`Compactor`).
   - Appends results as user messages with `ToolCallID` set, preserving the reasoning chain. The loop exits when the model returns zero tool calls.
   - Each turn dumps the full context history to `session.json` for debugging.
   - `RunSub()` provides isolated sub-agent execution (max 10 turns, read-only registry, returns text summary).
   - `Session` (`session.go`): thread-safe history with `sync.RWMutex`. `GetWorkingMemory(limit)` slices the last N messages with orphan ToolResult protection (skips leading orphan tool results to avoid API 400 errors). `SessionManager` + `GlobalSessionMgr` support multi-user/multi-terminal isolation.
   - `Reporter` interface (`reporter.go`): 4 callbacks (OnThinking, OnToolCall, OnToolResult, OnMessage) for pluggable output. `TerminalReporter` outputs with emoji and hierarchical indentation for sub-agents.
   - `ReminderInjector` (`reminder.go`): monitors consecutive tool call failures via MD5 fingerprinting. After 3 identical failures, injects a `RoleUser` intervention message to break dead loops.

3. **Context** (`internal/context/`) — dynamic system prompt assembly and safety mechanisms:
   - `PromptComposer` (`composer.go`): builds the system prompt from: core identity/rules, optional plan mode instructions, `AGENTS.md` project guide, blueprint index from `.claw/blueprint/*.md`, and dynamically loaded skills from `.claw/skills/*/SKILL.md`.
   - `Compactor` (`compactor.go`): prevents OOM by monitoring context length. When exceeding threshold (default 20,000 chars), masks old tool results and truncates large recent ones (head+tail preservation).
   - `RecoveryManager` (`recover.go`): injects actionable Chinese-language recovery hints on tool errors, prioritized by `ErrorCode` match with string-pattern fallback.
   - `SkillLoader` (`skill.go`): scans `.claw/skills/` directory, parses YAML frontmatter from `SKILL.md` files, and formats them for system prompt injection.
   - `BlueprintLoader` (`blueprint.go`): scans `.claw/blueprint/*.md` and injects only a compact index into the system prompt (file list + first-heading descriptions). The agent reads full blueprint files on-demand via `read_file` when it actually needs to work in a given directory, avoiding per-turn token waste.

4. **Provider** (`internal/provider/`) — abstracts LLM backends behind the `LLMProvider` interface (single method: `Generate(ctx, messages, tools) *schema.Message`).
   - Config loading (`API_KEY`, `baseURL`, `.env`) is shared in `config.go` via `loadConfig()`.
   - `OpenAIProvider` (`openai.go`): uses `openai-go/v3` SDK, translates internal `schema.Message` to OpenAI API format. Thinking mode disabled.
   - `ClaudeProvider` (`claude.go`): uses `anthropic-sdk-go`, sends system prompt separately, translates ToolUseBlock/ToolResultBlock.

5. **Tools** (`internal/tools/`) — `Registry` maps tool names to `BaseTool` implementations.
   - `BaseTool` interface: `Name()`, `Definition()` (JSON Schema), `Execute(ctx, args) (string, error)`.
   - Optional `LockHinter` interface: tools that implement it declare path-level lock requirements. Tools without it (bash) take the global write lock.
   - `MiddlewareFunc`: intercepts tool calls before lock acquisition. Used by feishu approval to block dangerous commands.
   - `ExecuteParallel`: concurrent execution via `sync.WaitGroup` + buffered channel semaphore (cap 5), preserving result order by index.
   - `PathLockManager` (`lockmgr.go`): two-tier locking — global `RWMutex` (bash vs file tools mutual exclusion) + per-path `RWMutex` pool (same-path serial, cross-path parallel). Paths are sorted/normalized to prevent deadlocks. `refCount` auto-cleans entries from the map.
   - 5 registered tools:
     - `read_file` — reads file content with `start_line`/`end_line` range. Truncated at 50KB.
     - `write_file` — creates or overwrites files. Auto-creates parent directories.
     - `edit_file` — exact string replacement. Two-pass algorithm: L1 exact match → L2 newline-normalized fallback. Returns structured errors (`ErrOldTextNotFound` / `ErrOldTextAmbiguous`).
     - `bash` — executes shell commands with 30s timeout. Output truncated at 30KB.
     - `spawn_subagent` (`subagent.go`) — delegates exploration to an isolated sub-agent with read-only tools (read_file + bash). Uses `AgentRunner` interface to avoid circular imports.

6. **Feishu** (`internal/feishu/`) — Feishu IM integration:
   - `FeishuBot` (`bot.go`): WebSocket-based bot that receives messages and routes them to `AgentEngine.Run`. `FeishuReporter` sends progress back to chat.
   - `ApprovalManager` (`approval.go`): sends interactive approval cards (approve/deny buttons) for dangerous commands. Auto-rejects on timeout. `IsDangerousCommand` uses regex to detect risky operations.

7. **Schema** (`internal/schema/`) — domain types shared across all layers:
   - `Message`: Role + Content + optional ToolCalls (assistant) / ToolCallID (user tool results).
   - `ToolCall`: ID, Name, Arguments (json.RawMessage).
   - `ToolResult`: ToolCallID, Output, ErrorCode, IsError.
   - `ToolError` (`errors.go`): structured error with `Code` (ErrorCode), `Message`, `Cause`. Implements `Unwrap()` for error chain support.

### Provider switching

The `LLMProvider` interface lets you swap backends without changing the engine or tools. Pass either `NewOpenAIProvider(model)` or `NewClaudeProvider(model)` when constructing the engine. Thinking mode is currently disabled via `option.WithJSONSet("thinking", ...)`.

### Tool sandboxing

All tools receive the engine's `workDir` and resolve paths relative to it. Bash commands run under a 30-second timeout, output truncated at 30KB. File reads are truncated at 50KB. The `PathLockManager` ensures safe concurrent access: same-path operations serialize, cross-path operations run in parallel, and bash operations globally exclude file operations.

### Self-healing mechanisms

Three built-in safeguards prevent the agent from getting stuck:
- **RecoveryManager**: on tool error, appends Chinese-language hints matched by error code.
- **ReminderInjector**: after 3 consecutive identical failures (fingerprinted by MD5 of normalized args), injects an intervention message.
- **Compactor**: when context exceeds 20,000 chars, compresses old history to prevent OOM.

### Auxiliary tools

- `cmd/split_log/` — standalone utility that splits a large log file into 100-line shards.
