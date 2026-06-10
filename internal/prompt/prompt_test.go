package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptComposerInjectsAgentsMDAndSkillIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("项目规则：错误提示使用中文"), 0644); err != nil {
		t.Fatalf("写 AGENTS.md 失败: %v", err)
	}

	skillDir := filepath.Join(dir, ".claw", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("创建技能目录失败: %v", err)
	}
	content := "---\nname: demo\ndescription: test skill\n---\nbody line"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("写 skill 失败: %v", err)
	}

	msg := NewPromptComposer(dir, false).Build()
	if !strings.Contains(msg.Content, "项目规则：错误提示使用中文") {
		t.Fatalf("system prompt should include AGENTS.md, got: %s", msg.Content)
	}
	if !strings.Contains(msg.Content, "demo") || !strings.Contains(msg.Content, "test skill") {
		t.Fatalf("system prompt should include skill index, got: %s", msg.Content)
	}
}

func TestBuildSystemPromptIncludesPlanModeWhenEnabled(t *testing.T) {
	out := BuildSystemPrompt(SystemPromptOptions{PlanMode: true})
	if !strings.Contains(out, "# 核心身份") {
		t.Fatalf("system prompt should include core prompt, got: %s", out)
	}
	if !strings.Contains(out, "Plan Mode: ON") {
		t.Fatalf("system prompt should include plan mode prompt, got: %s", out)
	}
}

func TestBuildSystemPromptSkipsPlanModeWhenDisabled(t *testing.T) {
	out := BuildSystemPrompt(SystemPromptOptions{PlanMode: false})
	if !strings.Contains(out, "# 核心身份") {
		t.Fatalf("system prompt should include core prompt, got: %s", out)
	}
	if strings.Contains(out, "Plan Mode: ON") {
		t.Fatalf("system prompt should not include plan mode prompt, got: %s", out)
	}
}

func TestSkillLoaderLoadOneMissingReturnsChineseError(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".claw", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("创建技能目录失败: %v", err)
	}
	content := "---\nname: demo\ndescription: test skill\n---\nbody line"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("写 skill 失败: %v", err)
	}

	_, err := NewSkillLoader(dir).LoadOne("missing")
	if err == nil {
		t.Fatal("expected missing skill error")
	}
	if !strings.Contains(err.Error(), "未找到技能") || !strings.Contains(err.Error(), "demo") {
		t.Fatalf("unexpected error: %v", err)
	}
}
