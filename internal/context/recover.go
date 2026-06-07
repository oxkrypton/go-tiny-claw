// internal/context/recovery.go
package context

import (
	"fmt"
	"strings"

	"github.com/oxkrypton/go-tiny-claw/internal/schema"
)

// RecoveryManager 负责在工具执行失败时，根据报错特征分析并注入恢复建议
type RecoveryManager struct{}

func NewRecoveryManager() *RecoveryManager {
	return &RecoveryManager{}
}

// AnalyzeAndInject 接收原始报错和可选的错误码，返回增强后的报错信息。
// 优先按 code 精确匹配，code 为空时回退到字符串特征匹配。
func (rm *RecoveryManager) AnalyzeAndInject(toolName string, rawError string, errCode schema.ErrorCode) string {
	var hint string

	// 第一优先级：按稳定错误码匹配
	if errCode != "" {
		hint = rm.hintByCode(errCode)
	} else {
		// 兜底：按字符串特征模糊匹配
		hint = rm.hintByPattern(toolName, rawError)
	}

	if hint == "" {
		return rawError
	}
	return fmt.Sprintf("%s\n\n[报错指南]: %s", rawError, hint)
}

// hintByCode 根据稳定的 ErrorCode 返回精确的恢复建议。
func (rm *RecoveryManager) hintByCode(code schema.ErrorCode) string {
	switch code {
	case schema.ErrInvalidArguments:
		return "你传递的参数格式不正确。请检查 JSON 字段名是否正确，以及是否遗漏了必需字段。"
	case schema.ErrFileNotFound:
		return "路径似乎不正确。请不要凭空猜测，先使用 `bash` 执行 `ls -la` 或 `find . -name` 命令查找正确的目录结构和文件名。"
	case schema.ErrPermissionDenied:
		return "你没有权限操作该文件。请检查工作区限制，或者思考是否需要修改其他文件。"
	case schema.ErrOldTextNotFound:
		return "你提供的 old_text 与文件当前内容不一致，或者缺少必要的缩进。请先使用 `read_file` 工具重新读取该文件，获取最新、准确的内容后，再重新发起编辑。"
	case schema.ErrOldTextAmbiguous:
		return "你的 old_text 不够具体，命中了多个相同代码块。请在 old_text 中增加上下相邻的几行代码，以确保替换的唯一性。"
	case schema.ErrCommandTimeout:
		return "该命令执行被超时强杀。如果它是一个常驻服务（如 server 或 watch），请使用 background:true 参数将其作为后台任务启动，避免阻塞主循环。"
	default:
		return ""
	}
}

// hintByPattern 保留原有字符串匹配逻辑作为兜底。
func (rm *RecoveryManager) hintByPattern(toolName string, rawError string) string {
	lowerError := strings.ToLower(rawError)

	switch toolName {
	case "edit_file":
		if strings.Contains(rawError, "在文件中未找到 old_text") || strings.Contains(rawError, "找不到该代码片段") {
			return "你提供的 old_text 与文件当前内容不一致，或者缺少必要的缩进。请先使用 `read_file` 工具重新读取该文件，获取最新、准确的内容后，再重新发起编辑。"
		} else if strings.Contains(rawError, "匹配到了多处") || strings.Contains(rawError, "提供更多上下文") {
			return "你的 old_text 不够具体，命中了多个相同代码块。请在 old_text 中增加上下相邻的几行代码，以确保替换的唯一性。"
		}

	case "read_file", "write_file":
		if strings.Contains(lowerError, "no such file or directory") {
			return "路径似乎不正确。请不要凭空猜测，先使用 `bash` 执行 `ls -la` 或 `find . -name` 命令查找正确的目录结构和文件名。"
		} else if strings.Contains(lowerError, "permission denied") {
			return "你没有权限操作该文件。请检查工作区限制，或者思考是否需要修改其他文件。"
		}

	case "bash":
		if strings.Contains(lowerError, "command not found") {
			return "系统中未安装该命令。请先思考：是否有替代命令？或者你需要先编写脚本进行安装？"
		} else if strings.Contains(rawError, "超时") || strings.Contains(rawError, "DeadlineExceeded") {
			return "该命令执行被超时强杀。如果它是一个常驻服务（如 server 或 watch），请使用 background:true 参数将其作为后台任务启动，避免阻塞主循环。"
		} else if strings.Contains(lowerError, "syntax error") {
			return "Bash 语法错误。请检查引号转义或特殊字符，确保命令在终端中可直接运行。"
		}
	}
	return ""
}
