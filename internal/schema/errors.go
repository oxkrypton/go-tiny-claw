package schema

import "fmt"

// ErrorCode 是工具层稳定输出的错误分类码，供 RecoveryManager 精确匹配。
type ErrorCode string

const (
	ErrInvalidArguments ErrorCode = "INVALID_ARGUMENTS"
	ErrFileNotFound     ErrorCode = "FILE_NOT_FOUND"
	ErrPermissionDenied ErrorCode = "PERMISSION_DENIED"
	ErrOldTextNotFound  ErrorCode = "OLD_TEXT_NOT_FOUND"
	ErrOldTextAmbiguous ErrorCode = "OLD_TEXT_AMBIGUOUS"
	ErrCommandTimeout   ErrorCode = "COMMAND_TIMEOUT"
)

// ToolError 是携带稳定错误码的工具错误，同时保留 Go 标准错误链能力。
type ToolError struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func NewToolError(code ErrorCode, message string, cause error) *ToolError {
	return &ToolError{Code: code, Message: message, Cause: cause}
}

func (e *ToolError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *ToolError) Unwrap() error {
	return e.Cause
}
