package context

// Skill 定义了从 SKILL.md 中解析出的标准化技能结构
type Skill struct {
	Name        string
	Description string
	Body        string //markdown 正文指令
}

// SkillLoader 负责从本地文件系统中加载并解析符合规范的技能模板
type SkillLoader struct {
	workDir string
}

func NewSkillLoader(workDir string) *SkillLoader {
	return &SkillLoader{workDir: workDir}
}

// LoadAll 扫描 .claw/skills 目录，解析所有 SKILL.md，并格式化为字符串准备注入 Context
