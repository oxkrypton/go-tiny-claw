package background

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestTaskManagerStartStatusLogsAndStop(t *testing.T) {
	dir := t.TempDir()
	manager := NewTaskManager(dir)
	defer manager.Cleanup()

	task, err := manager.Start("demo", "echo hello && sleep 30")
	if err != nil {
		t.Fatalf("启动后台任务失败: %v", err)
	}
	if task.TaskID != "demo" {
		t.Fatalf("期望 task_id 为 demo，实际为 %s", task.TaskID)
	}
	if !manager.IsAlive("demo") {
		t.Fatal("任务启动后应该处于运行状态")
	}

	time.Sleep(200 * time.Millisecond)
	data, err := os.ReadFile(task.LogFile)
	if err != nil {
		t.Fatalf("读取日志文件失败: %v", err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Fatalf("日志中应该包含命令输出，实际为 %s", string(data))
	}

	if err := manager.Stop("demo"); err != nil {
		t.Fatalf("停止后台任务失败: %v", err)
	}
	if manager.IsAlive("demo") {
		t.Fatal("任务停止后不应该仍然存活")
	}
}

func TestValidateTaskIDRejectsUnsafeValues(t *testing.T) {
	invalidIDs := []string{"", "a/b", "a\\b", "..", "../escape", strings.Repeat("x", 65)}
	for _, id := range invalidIDs {
		if err := ValidateTaskID(id); err == nil {
			t.Fatalf("期望 task_id %q 校验失败", id)
		}
	}
}
