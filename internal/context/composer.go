package context

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/oxkrypton/go-tiny-claw/internal/schema"
)

// PromptComposer 负责根据工作区环境动态生成 System Prompt
type PromptComposer struct {
	workDir        string
	planMode       bool
	blueprintLoader *BlueprintLoader
	skillLoader    *SkillLoader
}

func NewPromptComposer(workDir string, planMode bool) *PromptComposer {
	return &PromptComposer{
		workDir:        workDir,
		planMode:       planMode,
		blueprintLoader: NewBlueprintLoader(workDir),
		skillLoader:    NewSkillLoader(workDir),
	}
}

// Build 组装并返回一条完整的 RoleSystem 消息
func (c *PromptComposer) Build() schema.Message {
	var promptBuilder strings.Builder

	// 1. 极简内核 (Minimal Core) //
	// 确立基本身份与最底线的红线纪律
	promptBuilder.WriteString(`# 核心身份
你叫 Jarvis，一个由驾驭工程驱动的骨灰级研发助手。
你具备极简主义哲学，拒绝废话。你能通过系统提供的内置工具，创建、读取、修改和执行工作区中的代码。

# 核心纪律 (CRITICAL)
1. 如需检查文件是否存在，请使用 bash 的 ls 或 test -f。
2. 创建新文件时，务必使用 write_file，并同时提供 path 和 content 参数。
3. 编辑文件前务必先读取现有文件，以理解上下文。
4. 遇到工具执行报错时，仔细阅读 stderr，尝试自己修正命令并重试。
`)

	if c.planMode {
		promptBuilder.WriteString(`
# 长程任务与状态外部化强制规范 (Plan Mode: ON)

当你收到一条新指令被唤醒时，你必须、且只能按照以下【绝对顺序】执行你的动作：
**[STEP 1: 强制环境嗅探 (Bootstrapping)]**
- 收到指令后，你必须第一时间使用 bash (如: ` + "`ls -la`" + `) 检查当前工作区根目录下是否已经存在 ` + "`PLAN.md`" + ` 和 ` + "`TODO.md`" + `。
- **分支 A (全新任务)**：如果这两个文件不存在，说明这是一个全新的任务。你必须使用 write_file 依次创建它们：
1. 先创建 ` + "`PLAN.md`" + `，写下你的理解、架构设计、技术选型。
2. 再创建 ` + "`TODO.md`" + `，拆解出具体的可执行步骤（使用标准的 Markdown Checkbox 格式，如 ` + "`- [ ] 步骤1`" + `）。
- **分支 B (断点续传/任务唤醒)**：如果这两个文件已经存在，**绝对不要覆盖它们！** 这意味着系统刚刚重启，或者人类接管了进度。你必须立即使用 read_file 仔细阅读 ` + "`PLAN.md`" + ` 了解全局目标，并阅读 ` + "`TODO.md`" + ` 寻找第一个未被打勾的 ` + "`- [ ]`" + ` 任务，从那里直接继续干活。

**[STEP 2: 严格的单步执行与实时打勾]**
- 开始执行 ` + "`TODO.md`" + ` 中未完成的任务。
- **强制约束**：每当你通过 write_file 或 bash 真正完成了一个子任务后，你**必须立即停下来**，优先使用 edit_file 工具（或 bash 的 sed 命令），将 ` + "`TODO.md`" + ` 中对应的行修改为 ` + "`- [x]`" + `。
- 绝对不允许"一口气写完所有代码最后再打勾"。做完一步，必须打勾一步！

**[STEP 3: 迷失时的自救]**
- 如果你在执行中遇到了报错，或者不知道下一步该干嘛了，立即使用 read_file 重新读取 ` + "`TODO.md`" + ` 确认自己的位置。
`)
	}

	// 2. 外部化状态: 加载项目专属规范 (AGENTS.md)
	agentsMDPath := filepath.Join(c.workDir, "AGENTS.md")
	content, err := os.ReadFile(agentsMDPath)
	if err == nil {
		promptBuilder.WriteString("\n# 项目专属指南 (来自 AGENTS.md)")
		promptBuilder.WriteString(string(content))
		promptBuilder.WriteString("\n```\n")

	}

	// 3. 动态加载架构蓝图索引 (Blueprint)
	blueprintContent := c.blueprintLoader.LoadIndex()
	if blueprintContent != "" {
		promptBuilder.WriteString(blueprintContent)
	}

	// 4. 动态加载 skill 索引（渐进式披露：仅注入名称+触发条件，完整指令按需 read_file）
	skillContent := c.skillLoader.LoadIndex()
	if skillContent != "" {
		promptBuilder.WriteString(skillContent)
	}

	return schema.Message{
		Role:    schema.RoleSystem,
		Content: promptBuilder.String(),
	}
}
