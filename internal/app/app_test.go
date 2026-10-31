package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// 使用本地模型接口覆盖从启动装配到多轮请求的完整路径。
func TestRunConversationAndSessionIsolation(t *testing.T) {
	dir := configureTestEnvironment(t)
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type request struct {
		Model    string    `json:"model"`
		Messages []message `json:"messages"`
	}
	requests := make(chan request, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("读取模型请求失败：%v", err)
			http.Error(w, "请求格式错误", http.StatusBadRequest)
			return
		}
		requests <- req
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"local-test","object":"chat.completion","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"本地回复"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	}))
	defer server.Close()
	t.Setenv("baseURL", server.URL+"/v1")

	// 即便配置了服务，当前关闭的 MCP 注册也不应启动它。
	workDir := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(filepath.Join(workDir, ".claw"), 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(workDir, "mcp-started")
	mcpConfig, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
		"test": map[string]any{"command": "touch", "args": []string{marker}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".claw", "mcp.json"), mcpConfig, 0600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	options := Options{WorkDir: workDir, SessionID: "integration", InitialPrompt: "我叫小明"}
	if err := Run(context.Background(), options, strings.NewReader("我叫什么名字？\n/exit\n"), &output); err != nil {
		t.Fatal(err)
	}
	options.InitialPrompt = "新对话"
	if err := Run(context.Background(), options, strings.NewReader("/quit\n"), io.Discard); err != nil {
		t.Fatal(err)
	}

	if len(requests) != 3 {
		t.Fatalf("期望三轮模型请求，实际为 %d", len(requests))
	}
	for i, want := range [][]message{
		{{Role: "user", Content: "我叫小明"}},
		{{Role: "user", Content: "我叫小明"}, {Role: "assistant", Content: "本地回复"}, {Role: "user", Content: "我叫什么名字？"}},
		{{Role: "user", Content: "新对话"}},
	} {
		req := <-requests
		if req.Model != modelName || len(req.Messages) == 0 || req.Messages[0].Role != "system" {
			t.Fatalf("第 %d 次请求模型或系统消息不正确：%+v", i+1, req)
		}
		if !reflect.DeepEqual(req.Messages[1:], want) {
			t.Fatalf("第 %d 次请求上下文不正确：%+v", i+1, req.Messages[1:])
		}
	}
	if !strings.Contains(output.String(), workDir) || strings.Count(output.String(), "本轮完成") != 2 {
		t.Fatalf("工作目录或对话输出不正确：%s", output.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("关闭 MCP 注册时不应启动服务，标记文件检查结果：%v", err)
	}
	traces, err := filepath.Glob(filepath.Join(workDir, ".claw", "traces", "trace_integration_*.json"))
	if err != nil || len(traces) == 0 {
		t.Fatalf("会话结束后未保存运行记录：%v", err)
	}
}

func TestRunReturnsStartupFailure(t *testing.T) {
	dir := configureTestEnvironment(t)
	if err := os.MkdirAll(filepath.Join(dir, ".claw"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claw", "mcp.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), Options{}, strings.NewReader("/exit\n"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "初始化 MCP 配置失败") {
		t.Fatalf("期望返回中文启动错误，实际为 %v", err)
	}
}
