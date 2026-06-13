package mcpclient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config 对应工作区里的 .claw/mcp.json。
// v1 只读取 mcpServers，风格接近 Claude Desktop 的配置。
type Config struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// ServerConfig 描述一个 stdio MCP server 的启动方式。
type ServerConfig struct {
	Enable         *bool             `json:"enabled,omitempty"`
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
}

// IsEnabled 返回 server 是否启用。enabled 未配置时默认启用。
func (c ServerConfig) IsEnable() bool {
	if c.Enable == nil {
		return true
	}
	return *c.Enable
}

// Timeout 返回工具调用超时时间，单位秒。未配置时默认 30 秒。
func (c ServerConfig) Timeout() int {
	if c.TimeoutSeconds <= 0 {
		return 30
	}
	return c.TimeoutSeconds
}

// LoadConfig 从工作区读取 .claw/mcp.json。
// 文件不存在时返回空配置，不影响原有功能。
func LoadConfig(workDir string) (*Config, error) {
	path := filepath.Join(workDir, ".claw", "mcp.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{MCPServers: map[string]ServerConfig{}}, nil
		}
		return nil, fmt.Errorf("读取 MCP 配置失败: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析 MCP 配置失败，请检查 .claw/mcp.json 是否为合法 JSON: %w", err)
	}

	if cfg.MCPServers == nil {
		cfg.MCPServers = map[string]ServerConfig{}
	}

	return &cfg, nil
}
