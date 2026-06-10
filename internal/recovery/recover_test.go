package recovery

import (
	"fmt"
	"strings"
	"testing"

	"github.com/oxkrypton/go-tiny-claw/internal/schema"
)

func TestRecoveryManager_OldTextNotFound(t *testing.T) {
	rm := NewRecoveryManager()
	out := rm.AnalyzeAndInject("edit_file", "执行工具 edit_file 失败: [OLD_TEXT_NOT_FOUND] 在文件中未找到 old_text", schema.ErrOldTextNotFound)
	if !strings.Contains(out, "[报错指南]") {
		t.Fatal("expected recovery hint in output")
	}
	if !strings.Contains(out, "read_file") {
		t.Fatal("hint should mention read_file")
	}
}

func TestRecoveryManager_OldTextAmbiguous(t *testing.T) {
	rm := NewRecoveryManager()
	out := rm.AnalyzeAndInject("edit_file", "执行工具 edit_file 失败: [OLD_TEXT_AMBIGUOUS] 匹配到多处", schema.ErrOldTextAmbiguous)
	if !strings.Contains(out, "[报错指南]") {
		t.Fatal("expected recovery hint in output")
	}
	if !strings.Contains(out, "唯一性") || !strings.Contains(out, "old_text") {
		t.Fatal("hint should mention uniqueness and old_text, got:", out)
	}
}

func TestRecoveryManager_FileNotFound(t *testing.T) {
	rm := NewRecoveryManager()
	out := rm.AnalyzeAndInject("read_file", "执行工具 read_file 失败: [FILE_NOT_FOUND] 文件不存在", schema.ErrFileNotFound)
	if !strings.Contains(out, "ls -la") {
		t.Fatal("hint should mention ls -la, got:", out)
	}
}

func TestRecoveryManager_CommandTimeout(t *testing.T) {
	rm := NewRecoveryManager()
	out := rm.AnalyzeAndInject("bash", "执行工具 bash 失败: [COMMAND_TIMEOUT] 超时", schema.ErrCommandTimeout)
	if !strings.Contains(out, "background:true") {
		t.Fatal("hint should mention background:true, got:", out)
	}
}

func TestRecoveryManager_InvalidArguments(t *testing.T) {
	rm := NewRecoveryManager()
	out := rm.AnalyzeAndInject("read_file", "执行工具 read_file 失败: [INVALID_ARGUMENTS] 参数解析失败", schema.ErrInvalidArguments)
	if !strings.Contains(out, "JSON") || !strings.Contains(out, "参数格式") {
		t.Fatal("hint should mention JSON and format, got:", out)
	}
}

func TestRecoveryManager_PermissionDenied(t *testing.T) {
	rm := NewRecoveryManager()
	out := rm.AnalyzeAndInject("write_file", "执行工具 write_file 失败: [PERMISSION_DENIED] 无权限", schema.ErrPermissionDenied)
	if !strings.Contains(out, "权限") {
		t.Fatal("hint should mention permission, got:", out)
	}
}

func TestRecoveryManager_UnknownCodeReturnsOriginal(t *testing.T) {
	rm := NewRecoveryManager()
	out := rm.AnalyzeAndInject("bash", "执行工具 bash 失败: [UNKNOWN_CODE] 未知错误", schema.ErrorCode("UNKNOWN_CODE"))
	if !strings.Contains(out, "执行工具") {
		t.Fatal("unknown code should return original error text, got:", out)
	}
	// 没有匹配到 code hint，也没有匹配到兜底 pattern，应该原样返回
	if strings.Contains(out, "[报错指南]") {
		t.Fatal("unknown code should not add recovery hint")
	}
}

func TestRecoveryManager_PlainErrorFallback(t *testing.T) {
	rm := NewRecoveryManager()
	plainErr := fmt.Errorf("permission denied")
	out := rm.AnalyzeAndInject("read_file", plainErr.Error(), "")
	if !strings.Contains(out, "[报错指南]") {
		t.Fatal("plain error should match pattern fallback")
	}
	if !strings.Contains(out, "权限") {
		t.Fatal("hint should mention permission, got:", out)
	}
}

func TestRecoveryManager_PlainErrorNoMatch(t *testing.T) {
	rm := NewRecoveryManager()
	plainErr := "这是一个从未见过的奇怪错误"
	out := rm.AnalyzeAndInject("bash", plainErr, "")
	if out != plainErr {
		t.Fatalf("unmatched plain error should be returned as-is, got: %s", out)
	}
}

func TestRecoveryManager_CodePriorityOverStringMatch(t *testing.T) {
	rm := NewRecoveryManager()
	// 报错文本里包含 "permission denied" 但 code 是 FILE_NOT_FOUND，
	// 应该走 code 路径（提示 ls -la），而不是字符串兜底（提示权限）
	out := rm.AnalyzeAndInject("read_file", "打开文件失败: permission denied", schema.ErrFileNotFound)
	if !strings.Contains(out, "ls -la") {
		t.Fatal("code should take priority, hint should mention ls -la, got:", out)
	}
	if strings.Contains(out, "权限") {
		t.Fatal("code path should not fall through to pattern matching")
	}
}
