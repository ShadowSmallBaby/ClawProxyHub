package luahost

import (
	"testing"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	lua "github.com/yuin/gopher-lua"
)

type structuredStream struct {
	pb.ClawPlugin_ChatServer
	event *pb.StreamEvent
}

func (s *structuredStream) Send(ev *pb.StreamEvent) error { s.event = ev; return nil }

func TestStructuredContentFields(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	stream := &structuredStream{}
	L.SetGlobal("stream", newStreamTable(L, stream))
	if err := L.DoString(`stream.content_delta({text="no",refusal=true,source="chat",block_id="b",annotations='[{"type":"url_citation"}]'})`); err != nil {
		t.Fatal(err)
	}
	d := stream.event.GetContentDelta()
	if d.Text != "no" || !d.Refusal || d.Source != "chat" || d.BlockId != "b" || d.Annotations == "" {
		t.Fatal(d)
	}
}
