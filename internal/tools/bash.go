package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/oxkrypton/go-tiny-claw/internal/schema"
)

// BashTool 执行 bash 命令，支持前台同步执行和后台任务管理
type BashTool struct {
	workDir   string
	bgManager *BackgroundTaskManager
}

// NewBashTool 创建一个新的 BashTool，接收共享的 BackgroundTaskManager
func NewBashTool(workDir string, bgManager *BackgroundTaskManager) *BashTool {
	return &BashTool{workDir: workDir, bgManager: bgManager}
}

func (t *BashTool) Name() string {
	return "bash"
}

func (t *BashTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name: t.Name(),
		Description: "在当前工作区执行 bash 命令。支持普通命令（同步等待结果）和后台任务管理。" +
			"启动常驻服务（web server、watch、tail、dev server 等）时必须使用 background:true，否则会阻塞主循环。" +
			"后台任务启动后可通过 action=status/logs/stop 管理。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "要执行的 bash 命令。action=run 时必填（默认即 run），list/status/logs/stop 时不需要。",
				},
				"background": map[string]interface{}{
					"type":        "boolean",
					"description": "是否作为后台任务启动，默认 false。启动常驻服务时必须设为 true，工具会立即返回不阻塞。",
				},
				"task_id": map[string]interface{}{
					"type":        "string",
					"description": "后台任务标识符。启动时可选（不传自动生成），status/logs/stop 时必填。",
				},
				"action": map[string]interface{}{
					"type":        "string",
					"description": "操作类型：run（执行命令，默认）、list（列出所有后台任务）、status（查看任务状态）、logs（查看任务日志）、stop（停止任务）。",
				},
				"lines": map[string]interface{}{
					"type":        "integer",
					"description": "action=logs 时返回日志末尾行数，默认 80。",
				},
			},
		},
	}
}

type bashArgs struct {
	Command    string `json:"command"`
	Background bool   `json:"background"`
	TaskID     string `json:"task_id"`
	Action     string `json:"action"`
	Lines      int    `json:"lines"`
}

func (t *BashTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input bashArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", schema.NewToolError(schema.ErrInvalidArguments, "参数解析失败", err)
	}

	// action 默认为 run
	if input.Action == "" {
		input.Action = "run"
	}
	if input.Lines <= 0 {
		input.Lines = 80
	}

	switch input.Action {
	case "run":
		if input.Background {
			return t.runBackground(input)
		}
		return t.runForeground(ctx, input)
	case "list":
		return t.listTasks()
	case "status":
		return t.taskStatus(input.TaskID)
	case "logs":
		return t.taskLogs(input.TaskID, input.Lines)
	case "stop":
		return t.stopTask(input.TaskID)
	default:
		return "", schema.NewToolError(schema.ErrInvalidArguments,
			fmt.Sprintf("不支持的操作类型: %s，可选值为 run/list/status/logs/stop", input.Action), nil)
	}
}

// runForeground 同步执行命令，保持原有行为
func (t *BashTool) runForeground(ctx context.Context, input bashArgs) (string, error) {
	if input.Command == "" {
		return "", schema.NewToolError(schema.ErrInvalidArguments, "action=run 时必须提供 command 参数", nil)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "bash", "-c", input.Command)
	cmd.Dir = t.workDir

	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	if timeoutCtx.Err() == context.DeadlineExceeded {
		return "", schema.NewToolError(schema.ErrCommandTimeout,
			outputStr+"\n[警告: 命令执行超时(30s)，已被系统强制终止。"+
				"如果是启动常驻服务，请使用 background:true 参数将其作为后台任务启动，避免阻塞主循环。]", nil)
	}

	if err != nil {
		return fmt.Sprintf("执行报错: %v\n输出:\n%s", err, outputStr), nil
	}

	if outputStr == "" {
		return "命令执行成功, 无终端输出.", nil
	}

	const maxLen = 30000
	if len(outputStr) > maxLen {
		return fmt.Sprintf("%s\n\n...[终端输出过长，已截断至前 %d 字节]...", outputStr[:maxLen], maxLen), nil
	}

	return outputStr, nil
}

// runBackground 启动后台任务，立即返回
func (t *BashTool) runBackground(input bashArgs) (string, error) {
	if input.Command == "" {
		return "", schema.NewToolError(schema.ErrInvalidArguments, "后台启动时必须提供 command 参数", nil)
	}

	task, err := t.bgManager.Start(input.TaskID, input.Command)
	if err != nil {
		return "", schema.NewToolError(schema.ErrCommandTimeout, fmt.Sprintf("后台任务启动失败: %v", err), err)
	}

	return fmt.Sprintf("后台任务已启动\n"+
		"- task_id: %s\n"+
		"- PID: %d\n"+
		"- 日志文件: %s\n"+
		"- 启动时间: %s\n\n"+
		"后续可通过以下 action 管理：\n"+
		"- status: {\"action\":\"status\",\"task_id\":\"%s\"}\n"+
		"- logs:   {\"action\":\"logs\",\"task_id\":\"%s\"}\n"+
		"- stop:   {\"action\":\"stop\",\"task_id\":\"%s\"}",
		task.TaskID, task.PID, task.LogFile, task.StartTime.Format(time.RFC3339),
		task.TaskID, task.TaskID, task.TaskID), nil
}

// listTasks 列出所有后台任务
func (t *BashTool) listTasks() (string, error) {
	tasks := t.bgManager.List()
	if len(tasks) == 0 {
		return "当前没有后台任务。", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 个后台任务：\n\n", len(tasks)))
	for i, task := range tasks {
		alive := t.bgManager.IsAlive(task.TaskID)
		statusStr := string(task.Status)
		if statusStr == "running" && !alive {
			statusStr = "exited"
		}
		sb.WriteString(fmt.Sprintf("%d. task_id: %s\n   PID: %d\n   状态: %s\n   命令: %s\n   日志: %s\n   启动: %s\n",
			i+1, task.TaskID, task.PID, statusStr, task.Command, task.LogFile, task.StartTime.Format(time.RFC3339)))
		if i < len(tasks)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String(), nil
}

// taskStatus 查看指定任务状态
func (t *BashTool) taskStatus(taskID string) (string, error) {
	if err := ValidateTaskID(taskID); err != nil {
		return "", schema.NewToolError(schema.ErrInvalidArguments,
			fmt.Sprintf("task_id 无效: %v", err), err)
	}

	task, ok := t.bgManager.Get(taskID)
	if !ok {
		return "", schema.NewToolError(schema.ErrInvalidArguments,
			fmt.Sprintf("task_id '%s' 不存在。可用 action=list 查看所有任务。", taskID), nil)
	}

	alive := t.bgManager.IsAlive(taskID)
	statusStr := string(task.Status)
	if statusStr == "running" && !alive {
		statusStr = "exited"
	}

	runtime := time.Since(task.StartTime).Round(time.Second)
	exitInfo := ""
	if statusStr == "exited" {
		exitInfo = fmt.Sprintf("\n- 退出码: %d", task.ExitCode)
	}

	return fmt.Sprintf("任务 '%s' 状态：\n"+
		"- 状态: %s\n"+
		"- PID: %d\n"+
		"- 命令: %s\n"+
		"- 日志文件: %s\n"+
		"- 已运行: %s%s",
		taskID, statusStr, task.PID, task.Command, task.LogFile, runtime, exitInfo), nil
}

// taskLogs 读取后台任务日志末尾内容
func (t *BashTool) taskLogs(taskID string, lines int) (string, error) {
	if err := ValidateTaskID(taskID); err != nil {
		return "", schema.NewToolError(schema.ErrInvalidArguments,
			fmt.Sprintf("task_id 无效: %v", err), err)
	}

	task, ok := t.bgManager.Get(taskID)
	if !ok {
		return "", schema.NewToolError(schema.ErrInvalidArguments,
			fmt.Sprintf("task_id '%s' 不存在。可用 action=list 查看所有任务。", taskID), nil)
	}

	data, err := os.ReadFile(task.LogFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("任务 '%s' 的日志文件尚未生成: %s", taskID, task.LogFile), nil
		}
		return "", schema.NewToolError(schema.ErrPermissionDenied,
			fmt.Sprintf("读取日志文件失败: %v", err), err)
	}

	content := string(data)
	if content == "" {
		return fmt.Sprintf("任务 '%s' 的日志文件为空: %s", taskID, task.LogFile), nil
	}

	tailLines := strings.Split(content, "\n")
	if len(tailLines) > lines {
		tailLines = tailLines[len(tailLines)-lines:]
	}

	const maxLen = 30000
	out := strings.Join(tailLines, "\n")
	if len(out) > maxLen {
		out = out[len(out)-maxLen:]
	}
	return out, nil
}

// stopTask 停止一个后台任务
func (t *BashTool) stopTask(taskID string) (string, error) {
	if err := ValidateTaskID(taskID); err != nil {
		return "", schema.NewToolError(schema.ErrInvalidArguments,
			fmt.Sprintf("task_id 无效: %v", err), err)
	}

	task, ok := t.bgManager.Get(taskID)
	if !ok {
		return "", schema.NewToolError(schema.ErrInvalidArguments,
			fmt.Sprintf("task_id '%s' 不存在。可用 action=list 查看所有任务。", taskID), nil)
	}

	if err := t.bgManager.Stop(taskID); err != nil {
		return "", schema.NewToolError(schema.ErrCommandTimeout,
			fmt.Sprintf("停止失败: %v", err), err)
	}

	return fmt.Sprintf("后台任务 '%s' (PID: %d) 已停止。", task.TaskID, task.PID), nil
}
