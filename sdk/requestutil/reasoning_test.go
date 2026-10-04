package requestutil

import "testing"

func TestReasoningSources(t *testing.T) {
	for _, tc := range []struct {
		extra map[string]string
		want  string
	}{
		{nil, ""},
		{map[string]string{"responses_reasoning": `{"effort":"xhigh"}`}, "xhigh"},
		{map[string]string{"thinking": `{"type":"adaptive"}`, "anthropic_output_config": `{"effort":"low"}`}, "low"},
		{map[string]string{"thinking": `{"type":"disabled"}`}, "none"},
		{map[string]string{"thinking": `{"type":"disabled"}`, "anthropic_output_config": `{"effort":"high"}`}, "none"},
		{map[string]string{"thinking": `{"type":"disabled"}`, "reasoning_effort": "low"}, "low"},
		{map[string]string{"thinking": `{"type":"enabled","budget_tokens":4096}`}, "medium"},
		{map[string]string{"reasoning_effort": "low", "responses_reasoning": `{"effort":"high"}`}, "low"},
	} {
		if got := ReasoningEffort(tc.extra); got != tc.want {
			t.Fatalf("%v: got %q want %q", tc.extra, got, tc.want)
		}
	}
}
