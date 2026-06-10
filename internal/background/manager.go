package background

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// TaskStatus 表示后台任务的运行状态
type TaskStatus string

const (
	TaskRunning TaskStatus = "running"
	TaskExited  TaskStatus = "exited"
)

// Task 记录单个后台任务的元信息
type Task struct {
	TaskID    string
	PID       int
	Command   string
	StartTime time.Time
	LogFile   string
	Status    TaskStatus
	ExitCode  int

	cmd *exec.Cmd // 内部持有，用于 Stop 时杀进程组
}

// TaskManager 管理同一次 agent 进程内的所有后台任务
type TaskManager struct {
	mu      sync.Mutex
	tasks   map[string]*Task
	workDir string
	counter int
}

const maxTaskIDLen = 64

// NewTaskManager 创建一个新的后台任务管理器
func NewTaskManager(workDir string) *TaskManager {
	return &TaskManager{
		tasks:   make(map[string]*Task),
		workDir: workDir,
	}
}

// ValidateTaskID 对 task_id 做最低限度校验：拒绝空、含 / \ ..、超长
func ValidateTaskID(id string) error {
	if id == "" {
		return fmt.Errorf("task_id 不能为空")
	}
	if len(id) > maxTaskIDLen {
		return fmt.Errorf("task_id 长度超过限制（最大 %d 字符）", maxTaskIDLen)
	}
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return fmt.Errorf("task_id 不能包含 / \\ 或 ..")
	}
	return nil
}

// generateID 自动生成一个唯一的 task_id
func (m *TaskManager) generateID() string {
	m.counter++
	return fmt.Sprintf("task-%d", m.counter)
}

// Start 启动一个后台任务，立即返回其元信息
func (m *TaskManager) Start(taskID, command string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if taskID == "" {
		taskID = m.generateID()
	}
	if err := ValidateTaskID(taskID); err != nil {
		return nil, err
	}
	if _, exists := m.tasks[taskID]; exists {
		return nil, fmt.Errorf("task_id '%s' 已存在，请换一个名称或先停止旧任务", taskID)
	}

	logDir := filepath.Join(m.workDir, ".claw", "run")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	logFile := filepath.Join(logDir, taskID+".log")

	f, err := os.Create(logFile)
	if err != nil {
		return nil, fmt.Errorf("创建日志文件失败: %w", err)
	}

	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = m.workDir
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		f.Close()
		return nil, fmt.Errorf("启动后台任务失败: %w", err)
	}

	task := &Task{
		TaskID:    taskID,
		PID:       cmd.Process.Pid,
		Command:   command,
		StartTime: time.Now(),
		LogFile:   logFile,
		Status:    TaskRunning,
		cmd:       cmd,
	}
	m.tasks[taskID] = task

	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		task.Status = TaskExited
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				task.ExitCode = exitErr.ExitCode()
			} else {
				task.ExitCode = -1
			}
		}
		m.mu.Unlock()
		f.Close()
	}()

	return task, nil
}

// Get 按 task_id 获取任务，不存在时返回 false
func (m *TaskManager) Get(taskID string) (*Task, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[taskID]
	return task, ok
}

// List 返回当前所有后台任务的切片
func (m *TaskManager) List() []*Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		out = append(out, t)
	}
	return out
}

// Stop 终止指定后台任务的整个进程组
func (m *TaskManager) Stop(taskID string) error {
	m.mu.Lock()
	task, ok := m.tasks[taskID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("task_id '%s' 不存在", taskID)
	}

	if task.cmd == nil || task.cmd.Process == nil {
		m.mu.Unlock()
		return fmt.Errorf("后台任务 '%s' 没有可停止的进程", taskID)
	}

	if err := syscall.Kill(-task.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("停止后台任务失败: %w", err)
	}

	task.Status = TaskExited
	m.mu.Unlock()
	return nil
}

// IsAlive 检查进程是否仍存活
func (m *TaskManager) IsAlive(taskID string) bool {
	m.mu.Lock()
	task, ok := m.tasks[taskID]
	m.mu.Unlock()
	if !ok || task.Status != TaskRunning {
		return false
	}
	if task.cmd == nil || task.cmd.Process == nil {
		return false
	}
	return task.cmd.Process.Signal(syscall.Signal(0)) == nil
}

// Cleanup 停止所有正在运行的后台任务，进程退出前调用
func (m *TaskManager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, task := range m.tasks {
		if task.Status != TaskRunning || task.cmd == nil || task.cmd.Process == nil {
			continue
		}
		_ = syscall.Kill(-task.cmd.Process.Pid, syscall.SIGKILL)
		task.Status = TaskExited
	}
}
