// internal/tools/read_file.go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/oxkrypton/go-tiny-claw/internal/schema"
)

// ReadFileTool 实现了读取本地文件内容的工具
type ReadFileTool struct {
	//将引擎的 WorkDir 注入给工具, 限制他只能在此目录及其子目录下操作
	workDir string
}

func NewReadFileTool(workDir string) *ReadFileTool {
	return &ReadFileTool{workDir: workDir}
}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

// Definition 向大模型清晰地描述这个工具的用途和参数格式
func (t *ReadFileTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "读取指定路径的文件内容.请提供相对工作区的路径.",
		//遵循 JSON Schema 规范定义参数
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "要读取的文件路径, 如 cmd/claw/main.go",
				},
				"start_line": map[string]interface{}{
					"type":        "integer",
					"description": "起始行号（从 1 开始，包含该行）。不传则从第 1 行开始。",
				},
				"end_line": map[string]interface{}{
					"type":        "integer",
					"description": "结束行号（包含该行）。不传则读到文件末尾。",
				},
			},
			"required": []string{"path"},
		},
	}
}

// readFileArgs 内部定义用于反序列化的结构体
type readFileArgs struct {
	Path      string `json:"path"`
	StartLine *int   `json:"start_line"` // 指针, nil 表示未传
	EndLine   *int   `json:"end_line"`
}

func (t *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	// 1. 延迟解析：将大模型传过来的 JSON 参数解析为强类型结构体
	var input readFileArgs
	if err := json.Unmarshal(args, &input); err != nil {
		// 返回 error 会被 Registry 捕获并传给大模型，模型会知道自己 JSON 格式写错了
		return "", schema.NewToolError(schema.ErrInvalidArguments, "参数解析失败", err)
	}

	// 2. 拼接绝对路径 (注意：生产环境中需要做路径穿越检测防范，防止 ../../etc/passwd)
	fullPath := filepath.Join(t.workDir, input.Path)

	// 3. 执行物理 IO 操作
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", schema.NewToolError(schema.ErrFileNotFound, "文件不存在，请确认路径是否正确", err)
		}
		if os.IsPermission(err) {
			return "", schema.NewToolError(schema.ErrPermissionDenied, "没有权限读取该文件", err)
		}
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("读取文件内容失败: %w", err)
	}
	//按分行符确定行号
	lines := strings.Split(string(content), "\n")

	//确定方位 (默认全文)
	start := 1
	end := len(lines)

	if input.StartLine != nil {
		start = *input.StartLine
	}
	if input.EndLine != nil {
		end = *input.EndLine
	}

	// 边界校验
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return "", schema.NewToolError(schema.ErrInvalidArguments,
			fmt.Sprintf("start_line(%d) 不能大于 end_line(%d)", start, end), nil)
	}

	//切片取行 (行号从 1 开始, slice 索引从 0 开始)
	selected := lines[start-1 : end]
	result := strings.Join(selected, "\n")

	// 4. 【核心防线】长度截断保护
	// 为了防止大模型读取几百 MB 的日志文件导致 Context 瞬间爆炸 (OOM)，
	// 在工具内部直接进行物理截断。
	const maxLen = 50000
	if len(result) > maxLen {
		header := fmt.Sprintf("[文件: %s, 共 %d 行, 内容过长已截断]\n", input.Path, len(lines))
		return header + result[:maxLen], nil
	}

	// 带上行号信息, 方便后续 str_replace 修改
	header := fmt.Sprintf("[文件:%s, 行 %d-%d, 共 %d 行]\n", input.Path, start, end, len(lines))
	return header + result, nil
}

// LockHints 声明 read_file 只对目标 path 取读锁。
// 路径归一化：先 join 到 workDir 再 Clean，得到绝对路径作为 lock key，
// 避免 "./foo" 与 "foo" 这类等价字符串拿到不同的锁。
func (t *ReadFileTool) LockHints(args json.RawMessage) ([]LockRequest, error) {
	var input readFileArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, err
	}
	return []LockRequest{{
		Path: filepath.Clean(filepath.Join(t.workDir, input.Path)),
		Mode: LockRead,
	}}, nil
}
