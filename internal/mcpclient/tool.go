package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/oxkrypton/go-tiny-claw/internal/schema"
)

// MCPTool 把远端 MCP tool 包装成 tiny-claw 内部工具。
type MCPTool struct {
	publicName string
	serverName string
	remoteName string

	description string
	inputSchema map[string]any

	client  *rpcClient
	timeout time.Duration
}

func NewMCPTool(serverName string, remote remoteTool, client *rpcClient, timeout time.Duration) *MCPTool {
	publicName := "mcp_" + sanitizeName(serverName) + "_" + sanitizeName(remote.Name)

	inputSchema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}

	if len(remote.InputSchema) > 0 {
		_ = json.Unmarshal(remote.InputSchema, &inputSchema)
	}

	description := remote.Description
	if description == "" {
		description = fmt.Sprintf("调用 MCP server %s 提供的工具 %s。", serverName, remote.Name)
	}

	return &MCPTool{
		publicName:  publicName,
		serverName:  serverName,
		remoteName:  remote.Name,
		description: description,
		inputSchema: inputSchema,
		client:      client,
		timeout:     timeout,
	}
}

func (t *MCPTool) Name() string {
	return t.publicName
}

func (t *MCPTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.publicName,
		Description: "[MCP] " + t.description,
		InputSchema: t.inputSchema,
	}
}

func (t *MCPTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var arguments map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &arguments); err != nil {
			return "", schema.NewToolError(schema.ErrInvalidArguments, "MCP 工具参数不是合法 JSON 对象", err)
		}
	}
	if arguments == nil {
		arguments = map[string]any{}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	var result callToolResult
	err := t.client.call(timeoutCtx, "tools/call", map[string]any{
		"name":      t.remoteName,
		"arguments": arguments,
	}, &result)
	if err != nil {
		return "", schema.NewToolError(schema.ErrCommandTimeout, "调用 MCP 工具失败", err)
	}

	
	output := renderToolContent(result.Content)
	if output == "" {
		output = "MCP 工具执行完成，但没有返回文本内容。"
	}

	if result.IsError {
		return "", schema.NewToolError(schema.ErrInvalidArguments, "MCP 工具返回错误: "+output, nil)
	}

	return output, nil
}

func renderToolContent(contents []toolContent) string {
	var parts []string

	for _, item := range contents {
		switch item.Type {
		case "text":
			if item.Text != "" {
				parts = append(parts, item.Text)
			}
		case "image":
			parts = append(parts, "[MCP 返回了图片内容，当前版本暂不展示图片]")
		case "audio":
			parts = append(parts, "[MCP 返回了音频内容，当前版本暂不展示音频]")
		case "resource":
			parts = append(parts, "[MCP 返回了资源内容，当前版本暂不展开资源]")
		default:
			parts = append(parts, fmt.Sprintf("[MCP 返回了暂不支持的内容类型: %s]", item.Type))
		}
	}
	return strings.Join(parts, "\n")
}
