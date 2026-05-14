# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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

**go-tiny-claw** is a miniature AI coding agent that implements the ReAct (Reasoning + Acting) loop. It talks to an LLM (Claude or OpenAI), executes tool calls, and feeds results back in a loop until the model decides it's done.

### Layer model (top → bottom)

1. **Entry point** (`cmd/claw/main.go`) — wires the provider, tool registry, and engine together, then calls `eng.Run()` with a user prompt.
2. **Engine** (`internal/engine/loop.go`) — the core ReAct loop. Each turn has two phases:
   - *Phase 1 (Thinking)*: calls the LLM with `availableTools=nil`, forcing it to plan in plain text without tool access.
   - *Phase 2 (Action)*: calls the LLM again with the thinking trace in context and tools restored. The model can emit tool calls or finish with a text reply.
   - Tool results (Observations) are appended as user messages with `ToolCallID` set, preserving the reasoning chain. The loop exits when the model returns zero tool calls.
   - Each turn dumps the full context history to `session.json` for debugging.
3. **Provider** (`internal/provider/`) — abstracts LLM backends behind the `LLMProvider` interface (single method: `Generate`). Each implementation translates the internal `schema.Message` format into provider-specific request shapes and back. Config loading (`API_KEY`, `baseURL`, `.env`) is shared in `config.go` via `loadConfig()`.
4. **Tools** (`internal/tools/`) — `Registry` maps tool names to `BaseTool` implementations. `BaseTool` has three methods: `Name()`, `Definition()` (JSON Schema), and `Execute(args)`. Tools are sandboxed to `workDir`. `ExecuteParallel` runs tool calls concurrently via `errgroup`, preserving result order.
5. **Schema** (`internal/schema/message.go`) — domain types shared across layers: `Message` (role + content + optional tool calls/results), `ToolCall`, `ToolResult`, and `ToolDefinition`.

### Provider switching

The `LLMProvider` interface lets you swap backends without changing the engine or tools. Pass either `NewOpenAIProvider(model)` or `NewClaudeProvider(model)` when constructing the engine. Thinking mode (`enableThinking=true`) works the same on both — it withholds tools on the first call so the model plans before acting.

### Tool sandboxing

All tools receive the engine's `workDir` and resolve paths relative to it. Bash commands run under a 30-second timeout and output is truncated at 8000 bytes. File reads are also truncated at 8000 bytes to prevent context explosion.
