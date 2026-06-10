package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type stubRunner struct {
	prompt string
}

func (s *stubRunner) RunSub(ctx context.Context, taskPrompt string, readOnlyRegistry Registry, reporter interface{}) (string, error) {
	s.prompt = taskPrompt
	return "summary text", nil
}

func TestSubagentTool_DelegatesAndFormatsResult(t *testing.T) {
	runner := &stubRunner{}
	tool := NewSubagentTool(runner, NewRegistry(), nil)

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"task_prompt":"inspect this"}`))
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if runner.prompt != "inspect this" {
		t.Fatalf("没有传递正确的 task prompt: %s", runner.prompt)
	}
	if !strings.Contains(out, "summary text") {
		t.Fatalf("结果不符合预期: %s", out)
	}
}
