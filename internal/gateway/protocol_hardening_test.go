package gateway

import (
	"strings"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/anthropicup"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/openaiup"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/responsesup"
)

func TestUnsupportedContentRejected(t *testing.T) {
	for _, tc := range []struct {
		parse      func([]byte) (*pb.ChatRequest, error)
		body, want string
	}{
		{parseChatCompletions, `{"messages":[{"role":"user","content":[{"type":"text","text":"read"},{"type":"file","file":{"file_id":"x"}}]}]}`, "file"},
		{parseChatCompletions, `{"messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"x"}}]}]}`, "input_audio"},
		{parseChatCompletions, `{"messages":[{"role":"user","content":{"text":"lost"}}]}`, "content"},
		{parseResponsesRequest, `{"input":[{"role":"user","content":[{"type":"input_image","file_id":"x"}]}]}`, "file_id"},
		{parseResponsesRequest, `{"input":[{"type":"function_call_output","call_id":"c","output":[{"type":"input_file","file_id":"x"}]}]}`, "input_file"},
		{parseAnthropicRequest, `{"messages":[{"role":"user","content":[{"type":"document","source":{"type":"url","url":"x"}}]}]}`, "document"},
		{parseAnthropicRequest, `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"c","content":[{"type":"tool_use","id":"nested"}]}]}]}`, "tool_use"},
		{parseAnthropicRequest, `{"messages":[{"role":"user","content":"hi"}],"tools":[{"type":"web_search_20250305","name":"web_search"}]}`, "hosted"},
	} {
		if _, err := tc.parse([]byte(tc.body)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("want %q, got %v for %s", tc.want, err, tc.body)
		}
	}
}

func TestAdaptiveThinkingRoundTrip(t *testing.T) {
	req, err := parseAnthropicRequest([]byte(`{"messages":[{"role":"user","content":"hi"}],"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if openaiup.ChatBody(req)["reasoning_effort"] != "high" {
		t.Fatal("Chat effort lost")
	}
	if responsesup.ChatBody(req)["reasoning"].(map[string]interface{})["effort"] != "high" {
		t.Fatal("Responses effort lost")
	}
	body := anthropicup.ChatBody(req)
	if body["thinking"].(map[string]interface{})["type"] != "adaptive" || body["output_config"].(map[string]interface{})["effort"] != "high" {
		t.Fatal(body)
	}
}

func TestToolHistoryValidation(t *testing.T) {
	for _, body := range []string{
		`{"messages":[{"role":"assistant","tool_calls":[{"id":"c","type":"function","function":{"name":"f","arguments":"null"}}]}]}`,
		`{"messages":[{"role":"assistant","tool_calls":[{"id":"c","type":"function","function":{"arguments":"{}"}}]}]}`,
		`{"messages":[{"role":"assistant","tool_calls":[{"id":"c","type":"function","function":{"name":"f","arguments":"{}"}},{"id":"c","type":"function","function":{"name":"g","arguments":"{}"}}]}]}`,
		`{"messages":[{"role":"tool","content":"result"}]}`,
	} {
		if _, err := parseChatCompletions([]byte(body)); err == nil {
			t.Fatalf("invalid history accepted: %s", body)
		}
	}
}

func TestLateToolNameAcrossOutputs(t *testing.T) {
	events := compatibilityEvents(t,
		compatibilityTool("c1", "", "{"),
		compatibilityTool("c2", "second", "{}"),
		compatibilityTool("c1", "first", "}"),
		&pb.StreamEvent{Event: &pb.StreamEvent_MessageFinish{MessageFinish: &pb.MessageFinish{FinishReason: "tool_calls"}}},
	)
	for _, protocol := range []string{"messages", "chat_completions", "responses"} {
		encoder := newEncoder(protocol, "model")
		out := ""
		for _, ev := range events {
			if ev.GetTaskFailed() != nil {
				t.Fatal(ev)
			}
			out += encoder.convertEvent(ev)
		}
		if !strings.Contains(out, `"name":"first"`) || !strings.Contains(out, `"name":"second"`) {
			t.Fatalf("%s lost name: %s", protocol, out)
		}
	}
}
