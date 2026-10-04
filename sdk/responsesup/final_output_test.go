package responsesup

import (
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"testing"
)

func TestFinalOutputFallbackAndDedup(t *testing.T) {
	final := `data: {"type":"response.completed","response":{"output":[{"type":"message","id":"m","content":[{"type":"output_text","text":"hello"}]},{"type":"function_call","id":"f","call_id":"c","name":"run","arguments":"{}"}]}}`
	for _, partial := range []bool{false, true} {
		var lines []string
		if partial {
			lines = append(lines,
				`data: {"type":"response.output_text.delta","item_id":"m","delta":"hel"}`,
				`data: {"type":"response.output_text.done","item_id":"m","text":"hello"}`,
				`data: {"type":"response.output_item.done","item":{"type":"message","id":"m","content":[{"type":"output_text","text":"hello"}]}}`,
				`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"f","call_id":"c","name":"run"}}`,
				`data: {"type":"response.function_call_arguments.delta","item_id":"f","delta":"{"}`,
			)
		}
		lines = append(lines, final, final)
		events := collect(lines)
		text, args, names, finishes := "", "", 0, 0
		for _, event := range events {
			if event.GetTaskFailed() != nil {
				t.Fatal(event)
			}
			text += event.GetContentDelta().GetText()
			if tool := event.GetToolCallDelta(); tool != nil {
				args += tool.ArgumentsDelta
				if tool.Name != "" {
					names++
				}
			}
			if event.GetMessageFinish() != nil {
				finishes++
			}
		}
		if text != "hello" || args != "{}" || names != 1 || finishes != 1 {
			t.Fatalf("partial=%t text=%q args=%q names=%d finishes=%d", partial, text, args, names, finishes)
		}
	}
}

func TestFinalOutputConflictAndDisconnect(t *testing.T) {
	for _, terminal := range []string{
		`data: {"type":"response.output_text.done","item_id":"m","text":"different"}`,
		`data: {"type":"response.completed","response":{"output":[{"type":"web_search_call","id":"s"}]}}`,
		"",
	} {
		events := collect([]string{`data: {"type":"response.output_text.delta","item_id":"m","delta":"hello"}`, terminal})
		if events[len(events)-1].GetTaskFailed().GetError().GetCode() != 502 {
			t.Fatal(events)
		}
	}
}

func TestParallelToolFinalsAndLateName(t *testing.T) {
	events := collect([]string{
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"f1","call_id":"c1"}}`,
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"f2","call_id":"c2","name":"second"}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"f1","delta":"{"}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"f2","delta":"{}"}`,
		`data: {"type":"response.completed","response":{"output":[{"type":"function_call","id":"f1","call_id":"c1","name":"first","arguments":"{}"},{"type":"function_call","id":"f2","call_id":"c2","name":"second","arguments":"{}"}]}}`,
	})
	args := map[string]string{}
	names := map[string]string{}
	for _, event := range events {
		if event.GetTaskFailed() != nil {
			t.Fatal(event)
		}
		if tool := event.GetToolCallDelta(); tool != nil {
			args[tool.Id] += tool.ArgumentsDelta
			if tool.Name != "" {
				names[tool.Id] = tool.Name
			}
		}
	}
	if args["c1"] != "{}" || args["c2"] != "{}" || names["c1"] != "first" || names["c2"] != "second" {
		t.Fatalf("%v %v", args, names)
	}
}

func TestReasoningSnapshotOnce(t *testing.T) {
	var events []*pb.StreamEvent
	p := NewParser(func(ev *pb.StreamEvent) { events = append(events, ev) })
	p.Feed(`data: {"type":"response.output_item.done","item":{"type":"reasoning","id":"r","summary":[{"type":"summary_text","text":"think"}],"encrypted_content":"opaque"}}`)
	p.Feed(`data: {"type":"response.completed","response":{"output":[{"type":"reasoning","id":"r","summary":[{"type":"summary_text","text":"think"}],"encrypted_content":"opaque"}]}}`)
	text, sigs := "", 0
	for _, ev := range events {
		text += ev.GetReasoningDelta().GetText()
		if ev.GetReasoningDelta().GetSignature() != "" {
			sigs++
		}
	}
	if text != "think" || sigs != 1 {
		t.Fatalf("%q %d", text, sigs)
	}
}
