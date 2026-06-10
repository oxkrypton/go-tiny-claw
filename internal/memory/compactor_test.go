package memory

import (
	"strings"
	"testing"

	"github.com/oxkrypton/go-tiny-claw/internal/schema"
)

func TestCompactorKeepsContextWhenUnderLimit(t *testing.T) {
	c := NewCompactor(1000, 2)
	msgs := []schema.Message{
		{Role: schema.RoleSystem, Content: "system"},
		{Role: schema.RoleUser, Content: "short"},
	}

	got := c.Compact(msgs)
	if len(got) != len(msgs) {
		t.Fatalf("expected same message count, got %d", len(got))
	}
	if got[1].Content != "short" {
		t.Fatalf("unexpected content: %s", got[1].Content)
	}
}

func TestCompactorKeepsSystemAndMasksOldToolOutput(t *testing.T) {
	c := NewCompactor(10, 1)
	longOutput := strings.Repeat("a", 300)
	msgs := []schema.Message{
		{Role: schema.RoleSystem, Content: "system prompt"},
		{Role: schema.RoleUser, Content: longOutput, ToolCallID: "old-tool"},
		{Role: schema.RoleAssistant, Content: "recent"},
	}

	got := c.Compact(msgs)
	if got[0].Content != "system prompt" {
		t.Fatalf("system prompt should be kept, got %q", got[0].Content)
	}
	if !strings.Contains(got[1].Content, "早期的工具输出已被系统强制清理") {
		t.Fatalf("old tool output should be masked, got %q", got[1].Content)
	}
	if got[2].Content != "recent" {
		t.Fatalf("recent message should be kept, got %q", got[2].Content)
	}
}

func TestCompactorTruncatesRecentLongToolOutput(t *testing.T) {
	c := NewCompactor(10, 1)
	longOutput := strings.Repeat("a", 1200)
	msgs := []schema.Message{
		{Role: schema.RoleSystem, Content: "system prompt"},
		{Role: schema.RoleUser, Content: "old"},
		{Role: schema.RoleUser, Content: longOutput, ToolCallID: "recent-tool"},
	}

	got := c.Compact(msgs)
	if !strings.Contains(got[2].Content, "内容过长") {
		t.Fatalf("recent long tool output should be truncated, got %q", got[2].Content)
	}
	if len(got[2].Content) >= len(longOutput) {
		t.Fatalf("expected truncated output to be shorter than original")
	}
}
