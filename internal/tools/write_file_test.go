package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileTool_WritesContent(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFileTool(dir)

	out, err := tool.Execute(context.Background(), mustJSON(`{"path":"nested/out.txt","content":"hello"}`))
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if !strings.Contains(out, "成功将内容写入到文件") {
		t.Fatalf("输出不符合预期: %s", out)
	}

	data, err := os.ReadFile(filepath.Join(dir, "nested", "out.txt"))
	if err != nil {
		t.Fatalf("读取结果文件失败: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("文件内容不正确: %s", string(data))
	}
}

func TestWriteFileTool_LockHints(t *testing.T) {
	tool := NewWriteFileTool(t.TempDir())
	hints, err := tool.LockHints(mustJSON(`{"path":"a/b.txt","content":"x"}`))
	if err != nil {
		t.Fatalf("LockHints 失败: %v", err)
	}
	if len(hints) != 1 || hints[0].Mode != LockWrite {
		t.Fatalf("锁提示不正确: %#v", hints)
	}
}
