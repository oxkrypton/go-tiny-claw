package session

import (
	"testing"

	"github.com/oxkrypton/go-tiny-claw/internal/schema"
)

func TestSessionAppendAndGetWorkingMemory(t *testing.T) {
	s := NewSession("test", "/tmp/work")

	s.Append(
		schema.Message{Role: schema.RoleUser, Content: "one"},
		schema.Message{Role: schema.RoleAssistant, Content: "two"},
		schema.Message{Role: schema.RoleUser, Content: "three"},
	)

	got := s.GetWorkingMemory(2)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0].Content != "two" || got[1].Content != "three" {
		t.Fatalf("unexpected working memory: %#v", got)
	}

	got[0].Content = "mutated"
	again := s.GetWorkingMemory(0)
	if again[1].Content != "two" {
		t.Fatalf("working memory should return a copy, got %#v", again)
	}
}

func TestSessionGetWorkingMemoryDropsOrphanToolResult(t *testing.T) {
	s := NewSession("test", "/tmp/work")

	s.Append(
		schema.Message{Role: schema.RoleAssistant, Content: "old"},
		schema.Message{Role: schema.RoleUser, Content: "orphan result", ToolCallID: "call-1"},
		schema.Message{Role: schema.RoleAssistant, Content: "next"},
	)

	got := s.GetWorkingMemory(2)
	if len(got) != 1 {
		t.Fatalf("expected orphan tool result to be dropped, got %d messages", len(got))
	}
	if got[0].Content != "next" {
		t.Fatalf("unexpected remaining message: %#v", got[0])
	}
}

func TestSessionRecordUsage(t *testing.T) {
	s := NewSession("test", "/tmp/work")

	s.RecordUsage(10, 20, 0.03)
	s.RecordUsage(1, 2, 0.004)

	if s.TotalPromptTokens != 11 {
		t.Fatalf("unexpected prompt tokens: %d", s.TotalPromptTokens)
	}
	if s.TotalCompletionTokens != 22 {
		t.Fatalf("unexpected completion tokens: %d", s.TotalCompletionTokens)
	}
	if s.TotalCostCNY != 0.034 {
		t.Fatalf("unexpected cost: %f", s.TotalCostCNY)
	}
}
