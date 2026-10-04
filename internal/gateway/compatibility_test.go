package gateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func compatibilityTool(id, name, args string) *pb.StreamEvent {
	return &pb.StreamEvent{Event: &pb.StreamEvent_ToolCallDelta{ToolCallDelta: &pb.ToolCallDelta{Id: id, Name: name, ArgumentsDelta: args}}}
}

func compatibilityEvents(t *testing.T, events ...*pb.StreamEvent) []*pb.StreamEvent {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan *pb.StreamEvent, len(events))
	for _, ev := range events {
		in <- ev
	}
	close(in)
	var out []*pb.StreamEvent
	for ev := range normalizeEvents(ctx, in, "model") {
		out = append(out, ev)
	}
	return out
}

// 旧插件按顺序发无 ID 续块，新插件可携带 ID 交错输出，出口应保留两个调用。
func TestToolContinuationCompatibility(t *testing.T) {
	for _, legacy := range []bool{true, false} {
		name := "explicit_interleaved"
		events := []*pb.StreamEvent{
			compatibilityTool("call_1", "first", "{"),
			compatibilityTool("call_2", "second", "{"),
			compatibilityTool("call_1", "", `"a":1}`),
			compatibilityTool("call_2", "", `"b":2}`),
		}
		if legacy {
			name = "legacy_sequential"
			events = []*pb.StreamEvent{
				compatibilityTool("call_1", "first", "{"),
				compatibilityTool("", "", `"a":1}`),
				compatibilityTool("call_2", "second", "{"),
				compatibilityTool("", "", `"b":2}`),
			}
		}
		t.Run(name, func(t *testing.T) {
			events = append(events, &pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{MessageFinish: &pb.MessageFinish{FinishReason: "tool_calls"}}})
			enc := newOpenAISSEState()
			aggr := &openaiAggregate{model: "model"}
			type call struct{ id, name, args string }
			calls := map[int]call{}
			finished := false
			for _, ev := range compatibilityEvents(t, events...) {
				if failed := ev.GetTaskFailed(); failed != nil {
					t.Fatal(failed)
				}
				finished = finished || ev.GetMessageFinish() != nil
				aggr.feed(ev)
				for _, line := range strings.Split(enc.convertEvent(ev), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					var chunk struct {
						Choices []struct {
							Delta struct {
								Tools []struct {
									Index    int
									ID       string
									Function struct{ Name, Arguments string }
								} `json:"tool_calls"`
							}
						}
					}
					if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err != nil {
						t.Fatal(err)
					}
					for _, choice := range chunk.Choices {
						for _, tool := range choice.Delta.Tools {
							c := calls[tool.Index]
							c.id += tool.ID
							c.name += tool.Function.Name
							c.args += tool.Function.Arguments
							calls[tool.Index] = c
						}
					}
				}
			}
			if !finished || len(calls) != 2 || len(aggr.tools) != 2 {
				t.Fatalf("incomplete tools: finished=%v calls=%v", finished, calls)
			}
			for i, want := range []call{{"call_1", "first", `{"a":1}`}, {"call_2", "second", `{"b":2}`}} {
				if calls[i] != want {
					t.Errorf("stream tool %d = %+v, want %+v", i, calls[i], want)
				}
				if got := aggr.tools[want.id]; got == nil || got.name != want.name || got.input != want.args {
					t.Errorf("aggregate tool %s = %+v", want.id, got)
				}
			}
			if legacy && events[1].GetToolCallDelta().Id != "" {
				t.Fatal("normalization changed producer event")
			}
		})
	}
}

func TestToolContinuationWithoutHeadFails(t *testing.T) {
	events := compatibilityEvents(t, compatibilityTool("", "", "{}"))
	if len(events) == 0 || events[len(events)-1].GetTaskFailed().GetError().GetCode() != 502 {
		t.Fatalf("orphan continuation must fail: %v", events)
	}
}

func TestNormalizedStopSequence(t *testing.T) {
	events := compatibilityEvents(t, &pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{MessageFinish: &pb.MessageFinish{FinishReason: "stop_sequence", StopSequence: "END"}}})
	enc := newAnthSSEState("model")
	var output string
	for _, ev := range events {
		output += enc.convertEvent(ev)
	}
	if !strings.Contains(output, `"stop_reason":"stop_sequence"`) || !strings.Contains(output, `"stop_sequence":"END"`) {
		t.Fatalf("lost stop sequence: %s", output)
	}
}
