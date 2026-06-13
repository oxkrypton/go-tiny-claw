package mcpclient

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/oxkrypton/go-tiny-claw/internal/tools"
)

// Manager 管理所有 MCP server 连接。
type Manager struct {
	config  *Config
	clients []*rpcClient
}

func NewManager(worDir string) (*Manager, error) {
	cfg, err := LoadConfig(worDir)
	if err != nil {
		return nil, err
	}

	return &Manager{
		config: cfg,
	}, nil
}

// RegisterTools 连接配置中的 MCP servers，并把它们的 tools 注册到 tiny-claw。
func (m *Manager) Register(ctx context.Context, registry tools.Registry) error {
	for serverName, serverCfg := range m.config.MCPServers {
		if !serverCfg.IsEnable() {
			continue
		}

		if serverCfg.Command == "" {
			log.Printf("⚠️ MCP server %s 缺少 command，已跳过\n", serverName)
			continue
		}

		timeout := time.Duration(serverCfg.Timeout()) * time.Second

		// 启动 MCP 子进程
		client, err := newRPCClient(ctx, serverCfg.Command, serverCfg.Args, serverCfg.Env)
		if err != nil {
			log.Printf("⚠️ MCP server %s 启动失败: %v\n", serverName, err)
			continue
		}

		// MCP 协议握手
		if err := initialize(ctx, client, timeout); err != nil {
			log.Printf("⚠️ MCP server %s 初始化失败: %v\n", serverName, err)
			_ = client.close()
			continue
		}

		remoteTools, err := listTools(ctx, client, timeout)
		if err != nil {
			log.Printf("⚠️ MCP server %s 获取工具列表失败: %v\n", serverName, err)
			_ = client.close()
			continue
		}

		m.clients = append(m.clients, client)

		for _, remote := range remoteTools {
			tool := NewMCPTool(serverName, remote, client, timeout)
			registry.Register(tool)
		}
		log.Printf("[MCP] server %s 已加载 %d 个工具\n", serverName, len(remoteTools))
	}

	return nil
}

func initialize(ctx context.Context, client *rpcClient, timeout time.Duration) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var result initializeResult
	err := client.call(timeoutCtx, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "go-tiny-claw",
			"version": "0.1.0",
		},
	}, &result)
	if err != nil {
		return err
	}

	// MCP 规范要求 initialize 完成后发送 initialized notification。
	if err := client.notify("notifications/initialized", map[string]any{}); err != nil {
		return fmt.Errorf("发送 MCP initialized 通知失败: %w", err)
	}

	return nil
}

func listTools(ctx context.Context, client *rpcClient, timeout time.Duration) ([]remoteTool, error) {
	var all []remoteTool
	cursor := ""

	for {
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)

		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}

		var result listToolsResult
		err := client.call(timeoutCtx, "tools/list", params, &result)
		cancel()

		if err != nil {
			return nil, err
		}

		all = append(all, result.Tools...)

		if result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}

	return all, nil
}

// Close 关闭所有 MCP server 子进程。
func (m *Manager) Close() error {
	for _, client := range m.clients {
		_ = client.close()
	}
	return nil
}
