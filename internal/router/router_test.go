package router

import (
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func msg(role, text string) *pb.EnvelopeMessage {
	return &pb.EnvelopeMessage{Role: role, Text: text}
}

func TestFingerprintStableAcrossTurns(t *testing.T) {
	// 第二轮：前缀不变，追加了 assistant+user
	first := &pb.ChatRequest{Messages: []*pb.EnvelopeMessage{
		msg("system", "sys"),
		msg("user", "第一问"),
	}}
	second := &pb.ChatRequest{Messages: []*pb.EnvelopeMessage{
		msg("system", "sys"),
		msg("user", "第一问"),
		msg("assistant", "第一答"),
		msg("user", "第二问"),
	}}
	if Fingerprint(first) != Fingerprint(second) {
		t.Errorf("same conversation got different fingerprints: %s vs %s",
			Fingerprint(first), Fingerprint(second))
	}
}

func TestFingerprintDiffersByConversation(t *testing.T) {
	a := &pb.ChatRequest{Messages: []*pb.EnvelopeMessage{msg("user", "你好")}}
	b := &pb.ChatRequest{Messages: []*pb.EnvelopeMessage{msg("user", "再见")}}
	if Fingerprint(a) == Fingerprint(b) {
		t.Errorf("different conversations share fingerprint")
	}
}

func TestParseGroups(t *testing.T) {
	entries, err := parseGroups(`[{"group_id":1,"weight":70},{"group_id":2,"weight":30}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].GroupID != 1 || entries[0].Weight != 70 {
		t.Errorf("entries wrong: %+v", entries)
	}
}

func TestPickGroupRespectsWeights(t *testing.T) {
	entries := []model.RouteGroupEntry{{GroupID: 1, Weight: 100}, {GroupID: 2, Weight: 0}} // 权重 0 = 不参与
	counts := map[int64]int{}
	for i := 0; i < 200; i++ {
		counts[pickGroup(entries).GroupID]++
	}
	if counts[1] != 200 {
		t.Errorf("zero-weight group must never be picked: %+v", counts)
	}
	// 全为 0：退回第一个
	if pickGroup([]model.RouteGroupEntry{{GroupID: 7, Weight: 0}, {GroupID: 8, Weight: 0}}).GroupID != 7 {
		t.Error("all-zero should fall back to first entry")
	}
}

func TestParseGroupsWithModel(t *testing.T) {
	entries, err := parseGroups(`[{"group_id":1,"weight":70,"model":"deepseek-v4.1-flash"},{"group_id":2,"weight":30,"model":"DeepSeek-v4.1-flash-0907"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Model != "deepseek-v4.1-flash" || entries[1].Model != "DeepSeek-v4.1-flash-0907" {
		t.Errorf("per-group model mapping wrong: %+v", entries)
	}
}
