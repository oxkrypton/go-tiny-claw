package skill

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Skill 定义了从 SKILL.md 中解析出的标准化技能结构
type Skill struct {
	Name        string
	Description string
	Body        string //markdown 正文指令
}

// Loader 负责从本地文件系统中加载并解析符合规范的技能模板
type Loader struct {
	workDir string
}

func NewLoader(workDir string) *Loader {
	return &Loader{workDir: workDir}
}

// LoadIndex 扫描 .claw/skills 目录，仅注入技能名称与触发条件（渐进式披露的索引层）。
// 完整执行指令通过 skill 工具按需加载，避免每轮烧 token。
func (s *Loader) LoadIndex() string {
	skillBaseDir := filepath.Join(s.workDir, ".claw", "skills")

	if _, err := os.Stat(skillBaseDir); os.IsNotExist(err) {
		return ""
	}

	var skills []Skill
	_ = filepath.WalkDir(skillBaseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		skill := parseSkillMD(string(content))
		skills = append(skills, skill)
		return nil
	})

	if len(skills) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("\n### 可用专业技能 (Agent Skills)\n")
	builder.WriteString("以下技能可供使用。当你确认需要某个技能时，调用 skill 工具加载其完整执行指南：\n\n")

	for _, sk := range skills {
		builder.WriteString(fmt.Sprintf("- **%s** — %s\n", sk.Name, sk.Description))
	}

	return builder.String()
}

// LoadOne 按技能名称加载单个 SKILL.md 的完整正文（去除 YAML frontmatter）。
// 若未找到匹配的技能，error 中列出所有可用技能名，以供模型自愈重试。
func (s *Loader) LoadOne(name string) (string, error) {
	skillBaseDir := filepath.Join(s.workDir, ".claw", "skills")

	var allNames []string
	var found *Skill
	_ = filepath.WalkDir(skillBaseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		skill := parseSkillMD(string(content))
		allNames = append(allNames, skill.Name)
		if strings.EqualFold(skill.Name, name) {
			found = &skill
		}
		return nil
	})

	if found == nil {
		return "", fmt.Errorf("未找到技能 %q，可用技能: %s", name, strings.Join(allNames, "、"))
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("## %s\n\n", found.Name))
	builder.WriteString(fmt.Sprintf("**触发条件**: %s\n\n", found.Description))
	builder.WriteString("**执行指南**:\n")
	builder.WriteString(found.Body)
	return builder.String(), nil
}

// parseSkillMD 极简解析带有 YAML Frontmatter 的 Markdown 内容
func parseSkillMD(content string) Skill {
	skill := Skill{
		Name:        "Unknown Skill",
		Description: "No description provided.",
		Body:        content, // 默认将全量内容作为 body
	}

	// 简单解析 YAML Frontmatter (以 --- 包裹)
	if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
		parts := strings.SplitN(content, "---", 3)
		if len(parts) == 3 {
			frontmatter := parts[1]
			skill.Body = strings.TrimSpace(parts[2])

			// 逐行提取 metadata
			lines := strings.Split(frontmatter, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name:") {
					skill.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
				} else if strings.HasPrefix(line, "description:") {
					skill.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
				}
			}
		}
	}
	return skill
}
