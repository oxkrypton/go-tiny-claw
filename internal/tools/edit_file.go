package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oxkrypton/go-tiny-claw/internal/schema"
)

type EditFileTool struct {
	workDir string
}

func NewEditFileTool(workDir string) *EditFileTool {
	return &EditFileTool{workDir: workDir}
}

func (t *EditFileTool) Name() string {
	return "edit_file"
}

func (t *EditFileTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "对现有文件进行局部的字符串替换。这比重写整个文件更安全、更快速。请提供足够的 old_text 上下文以确保匹配的唯一性。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "要修改的文件路径",
				},
				"old_text": map[string]interface{}{
					"type":        "string",
					"description": "文件中原有的文本。必须包含足够的上下文（建议上下各多包含几行），以确保在文件中的唯一性。",
				},
				"new_text": map[string]interface{}{
					"type":        "string",
					"description": "要替换成的新文本",
				},
			},
			"required": []string{"path", "old_text", "new_text"},
		},
	}
}

func (t *EditFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input editFileArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", schema.NewToolError(schema.ErrInvalidArguments, "参数解析失败", err)
	}

	fullPath := filepath.Join(t.workDir, input.Path)

	//1. 读取原文件内容
	contentBytes, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", schema.NewToolError(schema.ErrFileNotFound, "文件不存在，请确认路径是否正确", err)
		}
		if os.IsPermission(err) {
			return "", schema.NewToolError(schema.ErrPermissionDenied, "没有权限读取该文件", err)
		}
		return "", fmt.Errorf("读取文件失败: %w", err)
	}
	originalContent := string(contentBytes)

	//2. 调用精确匹配替换算法
	newContent, err := Replace(originalContent, input.OldText, input.NewText)
	if err != nil {
		// ToolError 直接向上透传给 registry/recovery 处理；非 ToolError 兜底为普通 error
		var toolErr *schema.ToolError
		if errors.As(err, &toolErr) {
			return "", err
		}
		return "", fmt.Errorf("替换失败: %w", err)
	}

	//3. 将新内容安全地写回磁盘
	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		if os.IsPermission(err) {
			return "", schema.NewToolError(schema.ErrPermissionDenied, "写回文件失败", err)
		}
		return "", fmt.Errorf("写回文件失败: %w", err)
	}

	return fmt.Sprintf("✅ 成功修改文件: %s", input.Path), nil
}

type editFileArgs struct {
	Path    string `json:"path"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

// Replace 精确匹配替换算法
func Replace(originalContent, oldText, newText string) (string, error) {
	//L1:精确匹配
	count := strings.Count(originalContent, oldText)
	//只匹配到一处
	if count == 1 {
		return strings.Replace(originalContent, oldText, newText, 1), nil
	}
	//匹配到多处
	if count > 1 {
		return "", schema.NewToolError(schema.ErrOldTextAmbiguous,
			fmt.Sprintf("old_text 匹配到了 %d 处，请提供更多的上下文代码以确保唯一性", count), nil)
	}

	//L2:换行符归一化 (统一将 \r\n 转换为 \n)
	normalizedContent := strings.ReplaceAll(originalContent, "\r\n", "\n")
	normalizedOld := strings.ReplaceAll(oldText, "\r\n", "\n")

	count = strings.Count(normalizedContent, normalizedOld)
	if count == 1 {
		return strings.Replace(normalizedContent, normalizedOld, newText, 1), nil
	}
	// L2 之后
	if count > 1 {
		return "", schema.NewToolError(schema.ErrOldTextAmbiguous,
			fmt.Sprintf("old_text 匹配到了 %d 处，请提供更多的上下文代码以确保唯一性", count), nil)
	}

	// count == 0，L1 和 L2 都找不到
	return "", schema.NewToolError(schema.ErrOldTextNotFound,
		"在文件中未找到 old_text 指定的内容，请用 read_file 重新查看文件后再试", nil)
}

// LockHints 声明 edit_file 对目标 path 取独占写锁。
func (t *EditFileTool) LockHints(args json.RawMessage) ([]LockRequest, error) {
	var input editFileArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, err
	}
	return []LockRequest{{
		Path: filepath.Clean(filepath.Join(t.workDir, input.Path)),
		Mode: LockWrite,
	}}, nil
}
