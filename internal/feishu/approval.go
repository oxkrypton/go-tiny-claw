package feishu

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sync"
	"time"
)

type ApprovalResult struct {
	Allowed bool
	Reason  string
}

// ApprovalManager 统一管理当前正在等待人类审批的任务
type ApprovalManager struct {
	mu sync.RWMutex
	// Key 是用于审批的唯一 TaskID，Value 是接收审批结果的 Channel
	pendingTask map[string]chan ApprovalResult
}

// 全局单例，方便在 Registry Middleware 和 Feishu Webhook 之间共享状态
var GlovalApprovalMgr = &ApprovalManager{
	pendingTask: make(map[string]chan ApprovalResult),
}

// WaitForApproval 发送飞书交互卡片，并阻塞当前协程等待用户点击按钮或超时。
func (m *ApprovalManager) WaitForApproval(taskID string, toolName string, args string, reporter *FeishuReporter, timeout time.Duration) (bool, string) {
	// 1. 创建用于阻塞当前引擎协程的 channel (容量为 1 防止死锁)
	ch := make(chan ApprovalResult, 1)

	m.mu.Lock()
	m.pendingTask[taskID] = ch
	m.mu.Unlock()

	// 2. 构建飞书交互卡片并发送，用按钮代替文本口令，防止误操作
	if reporter != nil {
		reporter.sendCard(buildApprovalCard(taskID, toolName, args))
	} else {
		fmt.Printf("\n\033[31m[需要审批 TaskID: %s] 工具: %s, 参数: %s\033[0m\n", taskID, toolName, args)
	}

	log.Printf("[Approval] 已发送审批卡片 (TaskID: %s)，协程挂起等待 (超时: %v)...\n", taskID, timeout)

	// 3. 阻塞等待飞书卡片按钮点击，或超时自动拒绝
	select {
	case result := <-ch:
		m.cleanup(taskID)
		return result.Allowed, result.Reason
	case <-time.After(timeout):
		m.cleanup(taskID)
		log.Printf("[Approval] 审批超时 (TaskID: %s)，已自动拒绝\n", taskID)
		return false, fmt.Sprintf("审批超时(%v)，已自动拒绝", timeout)
	}
}

// cleanup 清理内存中的 pending channel，超时和正常完成统一走此路径防止泄漏。
func (m *ApprovalManager) cleanup(taskID string) {
	m.mu.Lock()
	delete(m.pendingTask, taskID)
	m.mu.Unlock()
}

// ResolveApproval 由飞书卡片按钮回调触发，向 channel 发送信号解开阻塞。
func (m *ApprovalManager) ResolveApproval(taskID string, allowed bool, reason string) {
	m.mu.RLock()
	ch, exists := m.pendingTask[taskID]
	m.mu.RUnlock()

	if exists {
		log.Printf("[Approval] 收到卡片审批结果 (TaskID: %s, Allowed: %v)\n", taskID, allowed)
		ch <- ApprovalResult{Allowed: allowed, Reason: reason}
	} else {
		log.Printf("[Approval] 找不到对应的 TaskID: %s, 可能已经超时或处理完毕\n", taskID)
	}
}

// buildApprovalCard 构建飞书交互卡片 JSON，包含批准/拒绝两个按钮。
// 按钮的 value 字段嵌入 taskID，回调时飞书会原样返回。
func buildApprovalCard(taskID, toolName, args string) string {
	card := map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": "操作审批",
			},
			"template": "red",
		},
		"elements": []map[string]interface{}{
			{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": fmt.Sprintf("⚠️ Agent 试图执行以下高危操作：\n\n**工具**：%s\n**参数**：`%s`\n\n任务 ID：`%s`", toolName, args, taskID),
				},
			},
			{
				"tag": "hr",
			},
			{
				"tag": "action",
				"actions": []map[string]interface{}{
					{
						"tag": "button",
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": "✅ 批准",
						},
						"type":  "primary",
						"value": map[string]interface{}{"taskID": taskID, "action": "approve"},
					},
					{
						"tag": "button",
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": "❌ 拒绝",
						},
						"type":  "danger",
						"value": map[string]interface{}{"taskID": taskID, "action": "reject"},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(card)
	return string(b)
}

// IsDangerousCommand 简单的正则检查黑名单，判断该工具调用是否需要审批
func IsDangerousCommand(toolName string, args string) bool {
	// 对于纯读取的工具,默认 YOLO 模式, 全部放行
	if toolName != "bash" {
		return false
	}

	// 针对 bash 的高危操作
	if toolName == "bash" {
		dangerousPatterns := []string{
			`rm\s+-r`, // 级联删除 
			`sudo\s+`, // 提权操作 
			`drop\s+`, // 数据库危险命令 
			`>.*\.go`, // 恶意覆盖源代码 
			`systemctl\s+`, // 拦截系统级服务管理 
			`kill\s+`, // 拦截杀进程操作
		}
		for _, p := range dangerousPatterns {
			matched, _ := regexp.MatchString(p, args)
			if matched {
				return true
			}
		}
	}
	return false
}
