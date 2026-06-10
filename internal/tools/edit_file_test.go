package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditFileTool_ReplaceSingleMatch(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "demo.txt")
	if err := os.WriteFile(target, []byte("hello old world"), 0644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}

	tool := NewEditFileTool(dir)
	out, err := tool.Execute(context.Background(), mustJSON(`{"path":"demo.txt","old_text":"old","new_text":"new"}`))
	if err != nil {
		t.Fatalf("编辑失败: %v", err)
	}
	if !strings.Contains(out, "成功修改文件") {
		t.Fatalf("输出不符合预期: %s", out)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("读取结果文件失败: %v", err)
	}
	if string(data) != "hello new world" {
		t.Fatalf("替换结果不正确: %s", string(data))
	}
}

func TestReplaceAmbiguousAndMissing(t *testing.T) {
	if _, err := Replace("foo bar foo", "foo", "baz"); err == nil {
		t.Fatal("应当报告歧义")
	}
	if _, err := Replace("hello world", "missing", "x"); err == nil {
		t.Fatal("应当报告未找到")
	}
}

func TestEditFileTool_LockHints(t *testing.T) {
	tool := NewEditFileTool(t.TempDir())
	hints, err := tool.LockHints(mustJSON(`{"path":"a/b.txt","old_text":"x","new_text":"y"}`))
	if err != nil {
		t.Fatalf("LockHints 失败: %v", err)
	}
	if len(hints) != 1 || hints[0].Mode != LockWrite {
		t.Fatalf("锁提示不正确: %#v", hints)
	}
}
