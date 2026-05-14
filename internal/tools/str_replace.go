package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oxkrypton/go-tiny-claw/internal/schema"
)

type StrReplaceTool struct {
	workDir string
}

func NewStrReplaceTool(workDir string) *StrReplaceTool {
	return &StrReplaceTool{workDir: workDir}
}

func (t *StrReplaceTool) Name() string {
	return "str_replace"
}

func (t *StrReplaceTool) Definition() schema.ToolDefinition {
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

type strReplaceArgs struct {
	Path    string `json:"path"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

// 精确匹配替换算法
func Replace(originalContent, oldText, newText string) (string, error) {
	//L1:精确匹配
	count := strings.Count(originalContent, oldText)
	//只匹配到一处
	if count == 1 {
		return strings.Replace(originalContent, oldText, newText, 1), nil
	}
	//匹配到多处
	if count > 1 {
		return "", fmt.Errorf("old_text 匹配到了 %d 处, 请提供更多的上下文代码以确保唯一性", count)
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
		return "", fmt.Errorf("old_text 匹配到了 %d 处, 请提供更多的上下文代码以确保唯一性", count)
	}

	// count == 0，L1 和 L2 都找不到
	return "", fmt.Errorf("在文件中未找到 old_text 指定的内容, 请用 read_file 重新查看文件后再试")
}

func (t *StrReplaceTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input strReplaceArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("参数解析失败 %w", err)
	}

	fullPath := filepath.Join(t.workDir, input.Path)

	//1. 读取原文件内容
	contentBytes, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败, 请确认路径是否正确: %w", err)
	}
	originalContent := string(contentBytes)

	//2. 调用精确匹配替换算法
	newContent, err := Replace(originalContent, input.OldText, input.NewText)
	if err != nil {
		// 【驾驭哲学】将具体的报错原因 (如匹配到多处) 原样返回，让大模型自行纠正
		return fmt.Sprintf("执行报错: %v\n", err), nil
	}

	//3. 将新内容安全地写回磁盘
	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("写回文件失败 %w", err)
	}

	return fmt.Sprintf("✅ 成功修改文件: %s", input.Path), nil
}
