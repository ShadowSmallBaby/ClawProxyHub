package openaiup

import "testing"

func TestParallelToolNameFragments(t *testing.T) {
	events := collect([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"get_"}},{"index":1,"id":"b","function":{"name":"run"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"weather"}},{"index":1,"function":{"arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	})
	names, args := map[string]string{}, map[string]string{}
	for _, ev := range events {
		if ev.GetTaskFailed() != nil {
			t.Fatal(ev)
		}
		if tool := ev.GetToolCallDelta(); tool != nil {
			names[tool.Id] += tool.Name
			args[tool.Id] += tool.ArgumentsDelta
		}
	}
	if names["a"] != "get_weather" || names["b"] != "run" || args["a"] != "{}" || args["b"] != "{}" {
		t.Fatalf("%v %v", names, args)
	}
}

func TestToolEmptyArgumentsAndIdentityConflict(t *testing.T) {
	events := collect([]string{`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"run"}}]},"finish_reason":"tool_calls"}]}`})
	if len(events) != 2 || events[0].GetToolCallDelta().GetArgumentsDelta() != "{}" {
		t.Fatal(events)
	}
	events = collect([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"run"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"b","function":{"arguments":"{}"}}]}}]}`,
	})
	if events[len(events)-1].GetTaskFailed() == nil {
		t.Fatal(events)
	}
}
