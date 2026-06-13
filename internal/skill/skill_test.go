package skill

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolLoadOne(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".claw", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("创建技能目录失败: %v", err)
	}
	content := "---\nname: demo\ndescription: test skill\n---\nbody line"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("写 skill 失败: %v", err)
	}

	tool := NewTool(dir)
	out, err := tool.Execute(context.Background(), mustJSON(`{"skill":"demo"}`))
	if err != nil {
		t.Fatalf("加载技能失败: %v", err)
	}
	if !strings.Contains(out, "demo") || !strings.Contains(out, "body line") {
		t.Fatalf("结果不符合预期: %s", out)
	}
}

func TestLoaderIndex(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".claw", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("创建技能目录失败: %v", err)
	}
	content := "---\nname: demo\ndescription: test skill\n---\nbody line"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("写 skill 失败: %v", err)
	}

	loader := NewLoader(dir)
	out := loader.LoadIndex()
	if !strings.Contains(out, "demo") || !strings.Contains(out, "test skill") {
		t.Fatalf("索引不符合预期: %s", out)
	}
}

func TestLoaderLoadOneMissingReturnsChineseError(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".claw", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("创建技能目录失败: %v", err)
	}
	content := "---\nname: demo\ndescription: test skill\n---\nbody line"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("写 skill 失败: %v", err)
	}

	_, err := NewLoader(dir).LoadOne("missing")
	if err == nil {
		t.Fatal("expected missing skill error")
	}
	if !strings.Contains(err.Error(), "未找到技能") || !strings.Contains(err.Error(), "demo") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func mustJSON(s string) json.RawMessage {
	return json.RawMessage(s)
}
