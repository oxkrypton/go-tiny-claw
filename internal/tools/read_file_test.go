package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileTool_ReadRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}

	tool := NewReadFileTool(dir)
	out, err := tool.Execute(context.Background(), mustJSON(`{"path":"sample.txt","start_line":2,"end_line":3}`))
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if !strings.Contains(out, "line2") || !strings.Contains(out, "line3") {
		t.Fatalf("结果不符合预期: %s", out)
	}
}

func TestReadFileTool_FileNotFound(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())
	_, err := tool.Execute(context.Background(), mustJSON(`{"path":"missing.txt"}`))
	if err == nil || !strings.Contains(err.Error(), "文件不存在") {
		t.Fatalf("应返回文件不存在错误，实际: %v", err)
	}
}

func TestReadFileTool_LockHints(t *testing.T) {
	dir := t.TempDir()
	tool := NewReadFileTool(dir)
	hints, err := tool.LockHints(mustJSON(`{"path":"a/b.txt"}`))
	if err != nil {
		t.Fatalf("LockHints 失败: %v", err)
	}
	if len(hints) != 1 || hints[0].Mode != LockRead {
		t.Fatalf("锁提示不正确: %#v", hints)
	}
}

func mustJSON(s string) json.RawMessage {
	return json.RawMessage([]byte(s))
}
