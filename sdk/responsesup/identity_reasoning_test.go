package responsesup

import (
	"testing"
)

func TestReasoningTextAndSummaryIndependent(t *testing.T) {
	events := collect([]string{
		`data: {"type":"response.reasoning_text.delta","item_id":"r","content_index":0,"delta":"raw"}`,
		`data: {"type":"response.reasoning_summary_text.delta","item_id":"r","summary_index":0,"delta":"summary"}`,
		`data: {"type":"response.reasoning_text.delta","item_id":"r","content_index":1,"delta":"second"}`,
		`data: {"type":"response.reasoning_text.done","item_id":"r","content_index":0,"text":"raw!"}`,
		`data: {"type":"response.reasoning_summary_text.done","item_id":"r","summary_index":0,"text":"summary!"}`,
		`data: {"type":"response.completed","response":{"output":[{"type":"reasoning","id":"r","summary":[{"type":"summary_text","text":"summary!"}],"content":[{"type":"reasoning_text","text":"raw!"},{"type":"reasoning_text","text":"second!"}]}]}}`,
	})
	text, finishes := "", 0
	for _, ev := range events {
		if ev.GetTaskFailed() != nil {
			t.Fatal(ev)
		}
		text += ev.GetReasoningDelta().GetText()
		if ev.GetMessageFinish() != nil {
			finishes++
		}
	}
	if text != "rawsummarysecond!!!" || finishes != 1 {
		t.Fatalf("text=%q finishes=%d", text, finishes)
	}
}

func TestToolIdentityFailures(t *testing.T) {
	for name, lines := range map[string][]string{
		"duplicate added": {
			`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"a","call_id":"c","name":"first"}}`,
			`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"b","call_id":"c","name":"second"}}`,
		},
		"duplicate final": {
			`data: {"type":"response.completed","response":{"output":[{"type":"function_call","id":"a","call_id":"c","name":"first"},{"type":"function_call","id":"b","call_id":"c","name":"second"}]}}`,
		},
		"missing final name": {
			`data: {"type":"response.completed","response":{"output":[{"type":"function_call","id":"a","call_id":"c","arguments":"{}"}]}}`,
		},
		"missing name without output": {
			`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"a","call_id":"c"}}`,
			`data: {"type":"response.completed","response":{}}`,
		},
		"blank name incomplete": {
			`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"a","call_id":"c","name":"  "}}`,
			`data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"}}}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			lines = append(lines, `data: {"type":"response.completed","response":{}}`)
			events := collect(lines)
			failures := 0
			for _, ev := range events {
				if ev.GetMessageFinish() != nil {
					t.Fatal("unexpected success", ev)
				}
				if ev.GetTaskFailed() != nil {
					failures++
					if ev.GetTaskFailed().GetError().GetCode() != 502 {
						t.Fatal(ev)
					}
				}
			}
			if failures != 1 {
				t.Fatalf("failures=%d", failures)
			}
		})
	}
}
