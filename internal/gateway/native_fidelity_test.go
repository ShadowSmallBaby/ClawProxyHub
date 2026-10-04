package gateway

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/anthropicup"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/openaiup"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/responsesup"
	"google.golang.org/protobuf/proto"
)

func TestNativeOptionsDoNotLeakAcrossProtocols(t *testing.T) {
	for _, tc := range []struct {
		name, request string
		parse         func([]byte) (*pb.ChatRequest, error)
		build         func(*pb.ChatRequest) map[string]interface{}
		foreign       func(*pb.ChatRequest) map[string]interface{}
	}{
		{"chat", `{"model":"m","messages":[{"role":"user","content":"hi"}],"store":false,"metadata":{"n":9007199254740993},"service_tier":"default","prompt_cache_key":"key","prompt_cache_retention":"24h","safety_identifier":"safe"}`, parseChatCompletions, openaiup.ChatBody, anthropicup.ChatBody},
		{"responses", `{"model":"m","input":"hi","store":false,"metadata":{"n":9007199254740993},"service_tier":"default","prompt_cache_key":"key","truncation":"disabled"}`, parseResponsesRequest, responsesup.ChatBody, openaiup.ChatBody},
		{"anthropic", `{"model":"m","messages":[{"role":"user","content":"hi"}],"metadata":{"user_id":"u","n":9007199254740993},"service_tier":"standard_only","cache_control":{"type":"ephemeral","ttl":"1h"}}`, parseAnthropicRequest, anthropicup.ChatBody, responsesup.ChatBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := tc.parse([]byte(tc.request))
			if err != nil {
				t.Fatal(err)
			}
			body := tc.build(req)
			var original map[string]json.RawMessage
			_ = json.Unmarshal([]byte(tc.request), &original)
			for _, key := range []string{"metadata", "service_tier", "store", "prompt_cache_key", "prompt_cache_retention", "safety_identifier", "truncation", "cache_control"} {
				if v, ok := original[key]; ok {
					got, _ := json.Marshal(body[key])
					var a, b interface{}
					da := json.NewDecoder(strings.NewReader(string(v)))
					da.UseNumber()
					_ = da.Decode(&a)
					db := json.NewDecoder(strings.NewReader(string(got)))
					db.UseNumber()
					_ = db.Decode(&b)
					aa, _ := json.Marshal(a)
					bb, _ := json.Marshal(b)
					if string(aa) != string(bb) {
						t.Fatalf("%s = %s want %s", key, bb, aa)
					}
					if _, ok := tc.foreign(req)[key]; ok {
						t.Fatalf("leaked %s", key)
					}
				}
			}
		})
	}
}

func TestImageDetailRoundTrip(t *testing.T) {
	for _, detail := range []string{"auto", "low", "high"} {
		req, err := parseChatCompletions([]byte(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.png","detail":"` + detail + `"}}]}]}`))
		if err != nil {
			t.Fatal(err)
		}
		raw, err := proto.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		req = &pb.ChatRequest{}
		if err = proto.Unmarshal(raw, req); err != nil {
			t.Fatal(err)
		}
		responses := responsesup.ChatBody(req)
		data, _ := json.Marshal(responses)
		back, err := parseResponsesRequest(data)
		if err != nil {
			t.Fatal(err)
		}
		if back.Messages[0].Parts[0].ImageDetail != detail {
			t.Fatal(back)
		}
		chat, _ := json.Marshal(openaiup.ChatBody(back))
		if !strings.Contains(string(chat), `"detail":"`+detail+`"`) {
			t.Fatal(string(chat))
		}
	}
	if _, err := parseResponsesRequest([]byte(`{"input":[{"role":"user","content":[{"type":"input_image","image_url":"https://example.com/x","detail":"invalid"}]}]}`)); err == nil {
		t.Fatal("invalid detail accepted")
	}
}

func TestAnnotatedHistoryReplay(t *testing.T) {
	for _, tc := range []struct {
		name, request, want string
		parse               func([]byte) (*pb.ChatRequest, error)
		build               func(*pb.ChatRequest) map[string]interface{}
	}{
		{"chat", `{"messages":[{"role":"assistant","content":"answer","annotations":[{"type":"url_citation","url_citation":{"url":"https://example.com","start_index":0,"end_index":6}}],"refusal":"cannot"}]}`, `"refusal":"cannot"`, parseChatCompletions, openaiup.ChatBody},
		{"responses", `{"input":[{"role":"assistant","content":[{"type":"output_text","text":"answer","annotations":[{"type":"url_citation","url":"https://example.com","start_index":0,"end_index":6}]},{"type":"refusal","refusal":"cannot"}]}]}`, `"refusal":"cannot"`, parseResponsesRequest, responsesup.ChatBody},
		{"anthropic", `{"messages":[{"role":"assistant","content":[{"type":"text","text":"answer","citations":[{"type":"web_search_result_location","url":"https://example.com","cited_text":"answer"}]}]}]}`, `"citations":[`, parseAnthropicRequest, anthropicup.ChatBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := tc.parse([]byte(tc.request))
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(tc.build(req))
			if !strings.Contains(string(raw), tc.want) || !strings.Contains(string(raw), "https://example.com") {
				t.Fatal(string(raw))
			}
		})
	}
}

// 经过 protobuf 序列化验证新增字段确实能跨插件 RPC 保留。
func TestStructuredOutputAcrossRPC(t *testing.T) {
	for _, tc := range []struct {
		protocol, frames string
		parser           func(func(*pb.StreamEvent)) sdk.SSEParser
		want             []string
	}{
		{"responses", `data: {"type":"response.output_text.delta","item_id":"a","delta":"first"}

data: {"type":"response.output_text.delta","item_id":"b","delta":"second"}

data: {"type":"response.output_text.annotation.added","item_id":"b","annotation":{"type":"url_citation","start_index":0,"end_index":6,"url":"https://example.com","title":"ref"}}

data: {"type":"response.refusal.delta","item_id":"c","delta":"cannot"}

data: {"type":"response.completed","response":{"output":[{"id":"a","type":"message","content":[{"type":"output_text","text":"first"}]},{"id":"b","type":"message","content":[{"type":"output_text","text":"second","annotations":[{"type":"url_citation","start_index":0,"end_index":6,"url":"https://example.com","title":"ref"}]}]},{"id":"c","type":"message","content":[{"type":"refusal","refusal":"cannot"}]}]}}

`, func(emit func(*pb.StreamEvent)) sdk.SSEParser { return responsesup.NewParser(emit) }, []string{`"refusal":"cannot"`, `"annotations":[{"end_index":6`, `"text":"second"`}},
		{"chat_completions", `data: {"choices":[{"delta":{"refusal":"cannot","annotations":[{"type":"url_citation","url_citation":{"start_index":0,"end_index":3,"url":"https://example.com","title":"ref"}}]},"finish_reason":"stop"}]}

data: [DONE]

`, func(emit func(*pb.StreamEvent)) sdk.SSEParser { return openaiup.NewParser(emit) }, []string{`"refusal":"cannot"`, `"annotations":[`}},
		{"messages", `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"first"}}

data: {"type":"content_block_stop","index":0}

data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":"second"}}

data: {"type":"content_block_delta","index":1,"delta":{"type":"citations_delta","citation":{"type":"web_search_result_location","url":"https://example.com","title":"ref","cited_text":"second"}}}

data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}

data: {"type":"message_stop"}

`, func(emit func(*pb.StreamEvent)) sdk.SSEParser { return anthropicup.NewParser(emit) }, []string{`"citations":[`, `"text":"second"`}},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			a := newAggregate(tc.protocol, "m")
			enc := newEncoder(tc.protocol, "m")
			var stream string
			p := tc.parser(func(ev *pb.StreamEvent) {
				raw, err := proto.Marshal(ev)
				if err != nil {
					t.Fatal(err)
				}
				copy := &pb.StreamEvent{}
				if err = proto.Unmarshal(raw, copy); err != nil {
					t.Fatal(err)
				}
				if copy.GetTaskFailed() != nil {
					t.Fatal(copy)
				}
				a.feed(copy)
				stream += enc.convertEvent(copy)
			})
			if err := sdk.ScanSSE(strings.NewReader(tc.frames), p); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(a.result())
			for _, want := range tc.want {
				if !strings.Contains(string(raw), want) {
					t.Fatalf("missing %s in %s", want, raw)
				}
			}
			if tc.protocol == "responses" {
				if strings.Count(string(raw), `"url_citation"`) != 1 {
					t.Fatal(string(raw))
				}
				if !strings.Contains(stream, "response.refusal.delta") || !strings.Contains(stream, "response.output_text.annotation.added") {
					t.Fatal(stream)
				}
				result := a.result()["output"].([]interface{})
				for _, item := range result {
					m := item.(map[string]interface{})
					if m["type"] != "message" {
						continue
					}
					for _, part := range m["content"].([]interface{}) {
						block := part.(map[string]interface{})
						if block["text"] == "second" && len(block["annotations"].([]json.RawMessage)) != 1 {
							t.Fatal(block)
						}
					}
				}
			}
		})
	}
}
