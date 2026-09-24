package gateway

import (
	"strings"
	"testing"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func tmsg(role, text string) *pb.EnvelopeMessage {
	return &pb.EnvelopeMessage{Role: role, Text: text}
}

func TestEstimateTokens(t *testing.T) {
	req := &pb.ChatRequest{Messages: []*pb.EnvelopeMessage{tmsg("user", "abcdefg")}} // 7 字节
	if got := estimateTokens(req, 3.5); got != 2 {                                   // 7/3.5 = 2
		t.Fatalf("estimateTokens = %d, want 2", got)
	}
}

func TestTruncateForWindow_UnknownWindowNoop(t *testing.T) {
	req := &pb.ChatRequest{Messages: []*pb.EnvelopeMessage{tmsg("user", "x")}}
	if r := truncateForWindow(req, 0, 0.9, 3.5); r != 0 {
		t.Fatalf("unknown window should not truncate, rounds=%d", r)
	}
}

func TestTruncateForWindow_ShrinksOversize(t *testing.T) {
	msgs := []*pb.EnvelopeMessage{tmsg("system", "sys")}
	for i := 0; i < 9; i++ {
		msgs = append(msgs, tmsg("user", strings.Repeat("a", 100)))
	}
	req := &pb.ChatRequest{Messages: msgs}
	before := len(req.Messages)
	rounds := truncateForWindow(req, 50, 0.9, 3.5) // 阈值 45 token ≈ 157 字节，远小于总量
	if rounds == 0 {
		t.Fatal("expected truncation rounds > 0")
	}
	if len(req.Messages) >= before {
		t.Fatalf("messages not reduced: before=%d after=%d", before, len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		t.Fatalf("first message must be preserved, got role=%q", req.Messages[0].Role)
	}
	if req.Messages[1].Text != truncateNotice {
		t.Fatal("truncate notice not injected after first message")
	}
}

func TestTruncateOnce_SkipsOrphanToolResult(t *testing.T) {
	// n=8 → keepLast=4, cutEnd=4；索引 4 恰为孤儿 tool 结果，应被跳到 5。
	msgs := []*pb.EnvelopeMessage{
		tmsg("system", "s"),
		tmsg("user", "u1"), tmsg("assistant", "a1"), tmsg("user", "u2"),
		{Role: "tool", ToolCallId: "call_x", Text: "result"},
		tmsg("assistant", "a2"), tmsg("user", "u3"), tmsg("assistant", "a3"),
	}
	req := &pb.ChatRequest{Messages: msgs}
	if !truncateOnce(req) {
		t.Fatal("truncateOnce should have cut")
	}
	// 保留区（notice 之后的首条）不应是孤儿 tool 结果。
	if req.Messages[2].Role == "tool" {
		t.Fatalf("orphan tool_result leaked to head of kept region: %+v", req.Messages[2])
	}
}

func TestTruncateOnce_ShortConversationNoop(t *testing.T) {
	req := &pb.ChatRequest{Messages: []*pb.EnvelopeMessage{
		tmsg("system", "s"), tmsg("user", "u"), tmsg("assistant", "a"), tmsg("user", "u2"),
	}}
	if truncateOnce(req) {
		t.Fatal("conversations with <=4 messages must not be truncated")
	}
}
