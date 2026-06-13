package mcpclient

import "encoding/json"

type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	ServerInfo      map[string]any `json:"serverInfo,omitempty"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
}

type listToolsResult struct {
	Tools      []remoteTool `json:"tools"`
	NextCursor string       `json:"nextCursor,omitempty"`
}

type remoteTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

type callToolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`

	// text content
	Text string `json:"text,omitempty"`

	// image/audio content
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`

	// resource content
	Resource any `json:"resource,omitempty"`
}