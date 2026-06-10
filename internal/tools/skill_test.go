package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxpkg "github.com/oxkrypton/go-tiny-claw/internal/context"
)

func TestSkillTool_LoadOne(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".claw", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("创建技能目录失败: %v", err)
	}
	content := "---\nname: demo\ndescription: test skill\n---\nbody line"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("写 skill 失败: %v", err)
	}

	tool := NewSkillTool(dir)
	out, err := tool.Execute(context.Background(), mustJSON(`{"skill":"demo"}`))
	if err != nil {
		t.Fatalf("加载技能失败: %v", err)
	}
	if !strings.Contains(out, "demo") || !strings.Contains(out, "body line") {
		t.Fatalf("结果不符合预期: %s", out)
	}
}

func TestSkillLoaderIndex(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".claw", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("创建技能目录失败: %v", err)
	}
	content := "---\nname: demo\ndescription: test skill\n---\nbody line"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("写 skill 失败: %v", err)
	}

	loader := ctxpkg.NewSkillLoader(dir)
	out := loader.LoadIndex()
	if !strings.Contains(out, "demo") || !strings.Contains(out, "test skill") {
		t.Fatalf("索引不符合预期: %s", out)
	}
}
