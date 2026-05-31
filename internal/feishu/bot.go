package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/oxkrypton/go-tiny-claw/internal/engine"
	"github.com/oxkrypton/go-tiny-claw/internal/schema"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

type FeishuBot struct {
	client    *lark.Client
	appID     string
	appSecret string
	engine    *engine.AgentEngine // 持有核心引擎引用
	sess      engine.Session
	r         *FeishuReporter
}

func NewFeishuBot(eng *engine.AgentEngine, sess engine.Session) *FeishuBot {
	appID := os.Getenv("FEISHU_APP_ID")
	appSecret := os.Getenv("FEISHU_APP_SECRET")

	if appID == "" || appSecret == "" {
		log.Fatal("请设置 FEISHU_APP_ID 和 FEISHU_APP_SECRET")
	}

	// 实例化飞书官方客户端
	client := lark.NewClient(appID, appSecret)

	return &FeishuBot{
		client:    client,
		appID:     appID,
		appSecret: appSecret,
		engine:    eng,
		sess:      sess,
	}
}

// Start 通过 WebSocket 长连接接收飞书事件，调用后阻塞直到连接关闭。
// 长连接模式无需公网 IP、无需配置加解密密钥。
func (b *FeishuBot) Start(ctx context.Context) error {
	// 长连接模式下 verifyToken 和 encryptKey 填空字符串，SDK 内部自行鉴权
	handler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			contentStr := *event.Event.Message.Content
			contentStr = strings.TrimPrefix(contentStr, `{"text":"`)
			contentStr = strings.TrimSuffix(contentStr, `"}`)

			chatId := *event.Event.Message.ChatId
			log.Printf("[Feishu] 收到会话 %s 信息: %s \n", chatId, contentStr)

			// 拦截人工审批的特殊口令
			if strings.HasPrefix(contentStr, "approve ") {
				taskID := strings.TrimPrefix(contentStr, "approve")
				taskID = strings.TrimSpace(taskID)
				// 唤醒挂起的引擎协程
				GlovalApprovalMgr.ResolveApproval(taskID, true, "管理员已批准该操作")
				log.Printf("[Feishu] 会话 %s: ✅ 已为您批准任务 %s", chatId, taskID)
				return nil
			}
			if strings.HasPrefix(contentStr, "reject ") {
				taskID := strings.TrimPrefix(contentStr, "reject")
				taskID = strings.TrimSpace(taskID)
				// 唤醒挂起的引擎协程, 并反馈拒绝理由
				GlovalApprovalMgr.ResolveApproval(taskID, false, "管理员已拒绝该操作")
				log.Printf("[Feishu] 会话 %s: 🚫 已拒绝任务 %s", chatId, taskID)
				return nil
			}

			// 长连接同样要求 3 秒内完成处理，否则会重推，因此必须异步
			go b.handleAgentRun(chatId, contentStr)

			return nil
		})

	cli := larkws.NewClient(b.appID, b.appSecret, larkws.WithEventHandler(handler))
	return cli.Start(ctx)
}

// 返回FeishuBot绑定的Reporter
func (b *FeishuBot) Reporter() *FeishuReporter {
	return b.r
}

// handleAgentRun 是连接飞书与底层引擎的桥梁
func (b *FeishuBot) handleAgentRun(chatId string, prompt string) {
	// 为当前聊天窗口实例化一个专属的 Reporter
	reporter := &FeishuReporter{
		client: b.client,
		chatId: chatId,
	}

	b.r = reporter
	b.sess.Append(schema.Message{Role: schema.RoleUser, Content: prompt})

	// 启动引擎
	err := b.engine.Run(context.Background(), &b.sess, reporter)
	if err != nil {
		reporter.sendMsg(fmt.Sprintf("❌ Agent 运行崩溃: %v", err))

	}
}

// ==========================================
// FeishuReporter: 将引擎的输出格式化后发给飞书
// ==========================================
type FeishuReporter struct {
	client *lark.Client
	chatId string
}

// sendMsg 封装了调用飞书 OpenAPI 发送卡片/文本的操作
func (r *FeishuReporter) sendMsg(text string) {
	// 构建文本信息内容
	textCotent := map[string]string{
		"text": text,
	}
	contentBytes, _ := json.Marshal(textCotent)
	contentStr := string(contentBytes)

	msgReq := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(r.chatId).
			MsgType(larkim.MsgTypeText).
			Content(contentStr).
			Build()).
		Build()

	_, _ = r.client.Im.Message.Create(context.Background(), msgReq)
}

func (r *FeishuReporter) OnThinking(ctx context.Context) {
	// 仅发一个轻量级提示，避免飞书刷屏
	r.sendMsg("🤔 模型正在慢思考 (Thinking)...")
}

func (r *FeishuReporter) OnToolCall(ctx context.Context, toolName string, args string) {
	r.sendMsg(fmt.Sprintf("🛠️ **正在执行工具**：`%s`\n参数：`%s`", toolName, args))
}

func (r *FeishuReporter) OnToolResult(ctx context.Context, toolName string, result string, isError bool) {
	if isError {
		r.sendMsg(fmt.Sprintf("⚠️ **执行报错** (%s)：\n%s", toolName, result))
	} else {
		// 成功时仅汇报成功，不刷全量日志
		r.sendMsg(fmt.Sprintf("✅ **执行成功** (%s)", toolName))
	}
}

func (r *FeishuReporter) OnMessage(ctx context.Context, content string) {
	// 将模型最终的纯文本回答发给用户
	r.sendMsg(content)
}

// 编译时类型检查：确保 FeishuReporter 实现了 Reporter 接口
var _ engine.Reporter = (*FeishuReporter)(nil)
