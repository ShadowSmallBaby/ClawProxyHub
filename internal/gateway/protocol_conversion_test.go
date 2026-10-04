package gateway

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/anthropicup"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/openaiup"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/responsesup"
)

func TestProtocolToolStrictMatrix(t *testing.T) {
	parsers := []struct {
		name  string
		parse func([]byte) (*pb.ChatRequest, error)
		body  string
	}{
		{"chat", parseChatCompletions, `{"messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"f","parameters":{"type":"object"},"strict":%s}}]}`},
		{"responses", parseResponsesRequest, `{"input":"hi","tools":[{"type":"function","name":"f","parameters":{"type":"object"},"strict":%s}]}`},
		{"anthropic", parseAnthropicRequest, `{"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"f","input_schema":{"type":"object"},"strict":%s}]}`},
	}
	builders := map[string]func(*pb.ChatRequest) map[string]interface{}{
		"chat": openaiup.ChatBody, "responses": responsesup.ChatBody, "anthropic": anthropicup.ChatBody,
	}
	for _, source := range parsers {
		for _, strict := range []bool{true, false} {
			for target, build := range builders {
				t.Run(fmt.Sprintf("%s/%s/%t", source.name, target, strict), func(t *testing.T) {
					req, err := source.parse([]byte(fmt.Sprintf(source.body, fmt.Sprint(strict))))
					if err != nil {
						t.Fatal(err)
					}
					b, err := json.Marshal(build(req))
					if err != nil {
						t.Fatal(err)
					}
					var body struct{ Tools []map[string]interface{} }
					if err := json.Unmarshal(b, &body); err != nil {
						t.Fatal(err)
					}
					tool := body.Tools[0]
					if target == "chat" {
						tool = tool["function"].(map[string]interface{})
					}
					if value, ok := tool["strict"].(bool); !ok || value != strict {
						t.Fatalf("strict lost: %s", b)
					}
				})
			}
		}
	}
}

func TestProtocolStructuredOutputRoundTrip(t *testing.T) {
	for _, format := range []string{
		`{"type":"json_object"}`,
		`{"type":"json_schema","json_schema":{"name":"result","description":"result schema","strict":false,"schema":{"type":"object","properties":{"id":{"type":"integer","const":9007199254740993}},"additionalProperties":false}}}`,
	} {
		req, err := parseChatCompletions([]byte(`{"messages":[{"role":"user","content":"hi"}],"response_format":` + format + `}`))
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(responsesup.ChatBody(req))
		if err != nil {
			t.Fatal(err)
		}
		req, err = parseResponsesRequest(body)
		if err != nil {
			t.Fatal(err)
		}
		got, err := json.Marshal(openaiup.ChatBody(req)["response_format"])
		if err != nil {
			t.Fatal(err)
		}
		var expected json.RawMessage = []byte(format)
		// 解码为 RawMessage 比较，避免测试本身把 schema 大整数舍入。
		var a, b interface{}
		da, db := json.NewDecoder(strings.NewReader(string(got))), json.NewDecoder(strings.NewReader(string(expected)))
		da.UseNumber()
		db.UseNumber()
		if err := da.Decode(&a); err != nil {
			t.Fatal(err)
		}
		if err := db.Decode(&b); err != nil {
			t.Fatal(err)
		}
		ja, _ := json.Marshal(a)
		jb, _ := json.Marshal(b)
		if string(ja) != string(jb) {
			t.Fatalf("format changed: got %s want %s", ja, jb)
		}
	}
}

func TestProtocolRequestControls(t *testing.T) {
	req, err := parseResponsesRequest([]byte(`{"input":"hi","reasoning":{"effort":"high","summary":"auto"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if openaiup.ChatBody(req)["reasoning_effort"] != "high" {
		t.Fatal("Responses effort lost in Chat conversion")
	}
	if anthropicup.ChatBody(req)["thinking"] == nil {
		t.Fatal("Responses effort lost in Anthropic conversion")
	}
	req, err = parseChatCompletions([]byte(`{"messages":[{"role":"user","content":"hi"}],"max_tokens":10,"max_completion_tokens":20,"parallel_tool_calls":false,"tools":[{"type":"function","function":{"name":"f","parameters":{}}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	body := openaiup.ChatBody(req)
	if body["max_completion_tokens"] != int32(20) || body["max_tokens"] != nil {
		t.Fatalf("token limit changed: %v", body)
	}
	choice := anthropicup.ChatBody(req)["tool_choice"].(map[string]interface{})
	if choice["type"] != "auto" || choice["disable_parallel_tool_use"] != true {
		t.Fatalf("parallel control lost: %v", choice)
	}
}

func TestProtocolRejectsUnsupportedChoices(t *testing.T) {
	for name, parse := range map[string]func([]byte) (*pb.ChatRequest, error){"chat": parseChatCompletions, "responses": parseResponsesRequest, "anthropic": parseAnthropicRequest} {
		for _, choice := range []string{`"invalid"`, `{"type":"unknown"}`, `{"type":"function"}`, `{"type":"tool"}`} {
			t.Run(name+choice, func(t *testing.T) {
				_, err := parse([]byte(`{"messages":[{"role":"user","content":"hi"}],"input":"hi","tool_choice":` + choice + `}`))
				if err == nil {
					t.Fatal("invalid tool_choice accepted")
				}
			})
		}
		if _, err := parse([]byte(`{"messages":[{"role":"user","content":"hi"}],"input":"hi","tool_choice":null}`)); err != nil {
			t.Fatalf("%s null choice: %v", name, err)
		}
	}
	if _, err := parseChatCompletions([]byte(`{"messages":[{"role":"user","content":"hi"}],"n":2}`)); err == nil {
		t.Fatal("multiple choices silently accepted")
	}
}
