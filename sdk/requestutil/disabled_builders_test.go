package requestutil_test

import (
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/anthropicup"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/openaiup"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/responsesup"
)

func TestDisabledThinkingAcrossBuilders(t *testing.T) {
	req := &pb.ChatRequest{Model: "test", Extra: map[string]string{
		"thinking":                `{"type":"disabled"}`,
		"anthropic_output_config": `{"effort":"high"}`,
	}}
	if got := openaiup.ChatBody(req)["reasoning_effort"]; got != "none" {
		t.Fatalf("Chat effort = %v", got)
	}
	if got := responsesup.ChatBody(req)["reasoning"].(map[string]interface{})["effort"]; got != "none" {
		t.Fatalf("Responses effort = %v", got)
	}
	if got := anthropicup.ChatBody(req)["thinking"].(map[string]interface{})["type"]; got != "disabled" {
		t.Fatalf("Anthropic thinking = %v", got)
	}
}
