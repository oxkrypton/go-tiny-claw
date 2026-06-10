package prompt

import (
	"github.com/oxkrypton/go-tiny-claw/internal/schema"
)

// PromptComposer 负责根据工作区环境动态生成 System Prompt
type PromptComposer struct {
	workDir     string
	planMode    bool
	skillLoader *SkillLoader
}

func NewPromptComposer(workDir string, planMode bool) *PromptComposer {
	return &PromptComposer{
		workDir:     workDir,
		planMode:    planMode,
		skillLoader: NewSkillLoader(workDir),
	}
}

// Build 组装并返回一条完整的 RoleSystem 消息
func (c *PromptComposer) Build() schema.Message {
	return schema.Message{
		Role: schema.RoleSystem,
		Content: BuildSystemPrompt(SystemPromptOptions{
			WorkDir:    c.workDir,
			PlanMode:   c.planMode,
			SkillIndex: c.skillLoader.LoadIndex(),
		}),
	}
}
