package skill

import (
	"context"
	"encoding/json"

	"github.com/oxkrypton/go-tiny-claw/internal/schema"
	"github.com/oxkrypton/go-tiny-claw/internal/tools"
)

type Tool struct {
	loader *Loader
}

func Register(registry tools.Registry, workDir string) {
	registry.Register(NewTool(workDir))
}

func NewTool(workDir string) *Tool {
	return &Tool{loader: NewLoader(workDir)}
}

func (t *Tool) Name() string {
	return "skill"
}

func (t *Tool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "加载指定技能的完整执行指南。当你根据技能描述判断某个技能适用于当前任务时，调用此工具获取其详细指令。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"skill": map[string]interface{}{
					"type":        "string",
					"description": "要加载的技能名称，与系统提示词中列出的技能名精确匹配。",
				},
			},
			"required": []string{"skill"},
		},
	}
}

func (t *Tool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Skill string `json:"skill"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", schema.NewToolError(schema.ErrInvalidArguments, "参数解析失败", err)
	}

	content, err := t.loader.LoadOne(input.Skill)
	if err != nil {
		return "", schema.NewToolError(schema.ErrInvalidArguments, err.Error(), err)
	}
	return content, nil
}
