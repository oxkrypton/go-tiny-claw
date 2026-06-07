package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBash_Foreground_Output(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	out, err := tool.Execute(context.Background(), jsonArg(`{"command":"echo hello world"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "hello world") {
		t.Fatalf("expected 'hello world' in output, got: %s", out)
	}
}

func TestBash_Foreground_Timeout(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	// 30s+ 的命令会触发超时
	_, err := tool.Execute(context.Background(), jsonArg(`{"command":"sleep 35"}`))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "background:true") {
		t.Fatalf("timeout error should mention background:true, got: %s", errStr)
	}
}

func TestBash_Background_ImmediateReturn(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	out, err := tool.Execute(context.Background(), jsonArg(`{"command":"sleep 10","background":true,"task_id":"test-sleep"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "已启动") {
		t.Fatalf("expected startup confirmation, got: %s", out)
	}
	if !strings.Contains(out, "test-sleep") {
		t.Fatalf("expected task_id in output, got: %s", out)
	}
	if !strings.Contains(out, "PID:") {
		t.Fatalf("expected PID in output, got: %s", out)
	}
	if !strings.Contains(out, "日志文件:") {
		t.Fatalf("expected log file path, got: %s", out)
	}

	// 清理
	bgMgr.Stop("test-sleep")
}

func TestBash_AutoGenerateTaskID(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	out, err := tool.Execute(context.Background(), jsonArg(`{"command":"sleep 3","background":true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "task-1") {
		t.Fatalf("expected auto-generated task_id 'task-1', got: %s", out)
	}

	bgMgr.Stop("task-1")
}

func TestBash_List_Empty(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	out, err := tool.Execute(context.Background(), jsonArg(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "没有后台任务") {
		t.Fatalf("expected 'no tasks' message, got: %s", out)
	}
}

func TestBash_List_WithTasks(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	bgMgr.Start("task-a", "sleep 5")
	bgMgr.Start("task-b", "sleep 5")
	defer bgMgr.Stop("task-a")
	defer bgMgr.Stop("task-b")

	out, err := tool.Execute(context.Background(), jsonArg(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "共 2 个") {
		t.Fatalf("expected 2 tasks, got: %s", out)
	}
	if !strings.Contains(out, "task-a") || !strings.Contains(out, "task-b") {
		t.Fatalf("expected both tasks listed, got: %s", out)
	}
}

func TestBash_Status_Running(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	tool.Execute(context.Background(), jsonArg(`{"command":"sleep 30","background":true,"task_id":"status-test"}`))
	defer bgMgr.Stop("status-test")

	out, err := tool.Execute(context.Background(), jsonArg(`{"action":"status","task_id":"status-test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "running") {
		t.Fatalf("expected running status, got: %s", out)
	}
	if !strings.Contains(out, "PID:") {
		t.Fatalf("expected PID, got: %s", out)
	}
}

func TestBash_Status_Exited(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	tool.Execute(context.Background(), jsonArg(`{"command":"echo done","background":true,"task_id":"short-task"}`))

	// 等待短任务结束
	time.Sleep(500 * time.Millisecond)

	out, err := tool.Execute(context.Background(), jsonArg(`{"action":"status","task_id":"short-task"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "exited") {
		t.Fatalf("expected exited status, got: %s", out)
	}
}

func TestBash_Logs_WithOutput(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	tool.Execute(context.Background(), jsonArg(`{"command":"echo hello-from-bg && echo line2","background":true,"task_id":"log-test"}`))
	time.Sleep(300 * time.Millisecond)

	out, err := tool.Execute(context.Background(), jsonArg(`{"action":"logs","task_id":"log-test","lines":80}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "hello-from-bg") {
		t.Fatalf("expected log content, got: %s", out)
	}
	if !strings.Contains(out, "line2") {
		t.Fatalf("expected second line, got: %s", out)
	}
}

func TestBash_Logs_DefaultLines(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	tool.Execute(context.Background(), jsonArg(`{"command":"echo testline","background":true,"task_id":"log-def"}`))
	time.Sleep(200 * time.Millisecond)

	out, err := tool.Execute(context.Background(), jsonArg(`{"action":"logs","task_id":"log-def"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "testline") {
		t.Fatalf("expected log content, got: %s", out)
	}
}

func TestBash_Stop_Running(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	tool.Execute(context.Background(), jsonArg(`{"command":"sleep 60","background":true,"task_id":"kill-me"}`))

	out, err := tool.Execute(context.Background(), jsonArg(`{"action":"stop","task_id":"kill-me"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "已停止") {
		t.Fatalf("expected stopped confirmation, got: %s", out)
	}

	if bgMgr.IsAlive("kill-me") {
		t.Fatal("task should no longer be alive after stop")
	}
}

func TestBash_Status_Nonexistent(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	_, err := tool.Execute(context.Background(), jsonArg(`{"action":"status","task_id":"no-such-task"}`))
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
	if !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestBash_Logs_Nonexistent(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	_, err := tool.Execute(context.Background(), jsonArg(`{"action":"logs","task_id":"no-such-task"}`))
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
	if !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestBash_Stop_Nonexistent(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	_, err := tool.Execute(context.Background(), jsonArg(`{"action":"stop","task_id":"no-such-task"}`))
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
	if !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestBash_InvalidAction(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	_, err := tool.Execute(context.Background(), jsonArg(`{"action":"foobar"}`))
	if err == nil {
		t.Fatal("expected error for invalid action")
	}
	if !strings.Contains(err.Error(), "不支持") {
		t.Fatalf("expected unsupported action error, got: %v", err)
	}
}

func TestBash_TaskID_RejectSpecialChars(t *testing.T) {
	invalidIDs := []string{"a/b", "a\\b", "..", "../escape"}
	for _, id := range invalidIDs {
		err := ValidateTaskID(id)
		if err == nil {
			t.Errorf("expected validation error for task_id '%s'", id)
		}
	}
}

func TestBash_TaskID_RejectEmpty(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	// status 不填 task_id 应该报错
	_, err := tool.Execute(context.Background(), jsonArg(`{"action":"status"}`))
	if err == nil {
		t.Fatal("expected error for missing task_id")
	}
}

func TestBash_Run_NoCommand(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	_, err := tool.Execute(context.Background(), jsonArg(`{"action":"run"}`))
	if err == nil {
		t.Fatal("expected error for run without command")
	}
	if !strings.Contains(err.Error(), "command") {
		t.Fatalf("expected command-related error, got: %v", err)
	}
}

func TestBash_Background_NoCommand(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	_, err := tool.Execute(context.Background(), jsonArg(`{"background":true}`))
	if err == nil {
		t.Fatal("expected error for background without command")
	}
	if !strings.Contains(err.Error(), "command") {
		t.Fatalf("expected command-related error, got: %v", err)
	}
}

func TestBash_ShortCommand_NoOutput(t *testing.T) {
	dir := t.TempDir()
	bgMgr := NewBackgroundTaskManager(dir)
	tool := NewBashTool(dir, bgMgr)

	out, err := tool.Execute(context.Background(), jsonArg(`{"command":"mkdir -p subdir"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "命令执行成功") {
		t.Fatalf("expected success message for silent command, got: %s", out)
	}
}

// jsonArg 辅助函数，生成 json.RawMessage
func jsonArg(s string) []byte {
	return []byte(s)
}
