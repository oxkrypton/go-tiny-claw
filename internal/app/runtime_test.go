package app

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"
)

func TestRuntimeCloseStopsBackgroundTasks(t *testing.T) {
	configureTestEnvironment(t)
	cfg, err := loadConfig(Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := newRuntime(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	task, err := rt.background.Start("cleanup", "exec sleep 30")
	if err != nil {
		t.Fatal(err)
	}
	pid := task.PID
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rt.Close(); err != nil {
		t.Fatalf("重复关闭资源应安全返回：%v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("关闭应用后后台任务仍在运行")
}
