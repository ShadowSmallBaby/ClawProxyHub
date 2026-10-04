package anthropicup

import (
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"testing"
)

func TestInitialThinkingAndHostedBlock(t *testing.T) {
	var events []*pb.StreamEvent
	p := NewParser(func(ev *pb.StreamEvent) { events = append(events, ev) })
	p.Feed(`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"initial","signature":"sig"}}`)
	p.Feed(`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`)
	if len(events) != 2 || events[0].GetReasoningDelta().GetText() != "initial" || events[0].GetReasoningDelta().GetSignature() != "sig" {
		t.Fatal(events)
	}
	events = nil
	p = NewParser(func(ev *pb.StreamEvent) { events = append(events, ev) })
	p.Feed(`data: {"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"x","name":"web_search"}}`)
	p.Feed(`data: {"type":"message_stop"}`)
	if len(events) != 1 || events[0].GetTaskFailed() == nil {
		t.Fatal(events)
	}
}
