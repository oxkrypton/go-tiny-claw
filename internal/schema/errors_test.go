package schema

import (
	"errors"
	"fmt"
	"testing"
)

func TestToolError_ErrorContainsChineseMessage(t *testing.T) {
	err := NewToolError(ErrFileNotFound, "文件不存在，请确认路径是否正确", fmt.Errorf("open foo: no such file"))
	msg := err.Error()
	if msg == "" {
		t.Fatal("Error() returned empty string")
	}
	// 中文 message 必须原样出现在 Error() 输出中
	if !containsChinese(msg) {
		t.Fatalf("expected Chinese message in Error() output, got: %s", msg)
	}
}

func TestToolError_ErrorWithoutCause(t *testing.T) {
	err := NewToolError(ErrOldTextNotFound, "在文件中未找到 old_text", nil)
	msg := err.Error()
	if msg != "[OLD_TEXT_NOT_FOUND] 在文件中未找到 old_text" {
		t.Fatalf("unexpected Error() output: %s", msg)
	}
}

func TestToolError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("原始错误")
	err := NewToolError(ErrFileNotFound, "文件不存在", cause)
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is should match cause via Unwrap")
	}
}

func TestToolError_ErrorsAs(t *testing.T) {
	err := NewToolError(ErrOldTextAmbiguous, "匹配到多处", nil)
	var toolErr *ToolError
	if !errors.As(err, &toolErr) {
		t.Fatal("errors.As should recognize ToolError")
	}
	if toolErr.Code != ErrOldTextAmbiguous {
		t.Fatalf("expected OLD_TEXT_AMBIGUOUS, got %s", toolErr.Code)
	}
}

func TestToolError_ErrorsAsOnWrapped(t *testing.T) {
	original := NewToolError(ErrCommandTimeout, "超时了", nil)
	wrapped := fmt.Errorf("执行工具 bash 失败: %w", original)
	var toolErr *ToolError
	if !errors.As(wrapped, &toolErr) {
		t.Fatal("errors.As should recognize ToolError through fmt.Errorf wrapping")
	}
	if toolErr.Code != ErrCommandTimeout {
		t.Fatalf("expected COMMAND_TIMEOUT, got %s", toolErr.Code)
	}
}

func TestToolError_PlainErrorNotRecognized(t *testing.T) {
	plain := fmt.Errorf("普通错误")
	var toolErr *ToolError
	if errors.As(plain, &toolErr) {
		t.Fatal("plain error should not be recognized as ToolError")
	}
}

func containsChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}
