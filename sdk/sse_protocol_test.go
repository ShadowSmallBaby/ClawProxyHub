package sdk

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/openaiup"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func TestSSEMultilineAndEOF(t *testing.T) {
	var events []*pb.StreamEvent
	p := openaiup.NewParser(func(ev *pb.StreamEvent) { events = append(events, ev) })
	body := ": heartbeat\r\nevent: chunk\r\ndata: {\r\ndata: \"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\r\n\r\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\r\n\r\ndata: [DONE]"
	if err := ScanSSE(strings.NewReader(body), p); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].GetContentDelta().GetText() != "hello" || events[1].GetMessageFinish() == nil {
		t.Fatalf("bad events: %v", events)
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("broken") }

func TestSSEBrokenPartialEvent(t *testing.T) {
	p := &capParser{}
	err := ScanSSE(io.MultiReader(strings.NewReader("data: {\n"), brokenReader{}), p)
	if err == nil || p.errCode != 502 || p.finished || len(p.lines) != 0 {
		t.Fatalf("partial event emitted: %+v, %v", p, err)
	}
}

func TestSSEMultilineLimit(t *testing.T) {
	p := &capParser{}
	if err := ScanSSE(strings.NewReader(strings.Repeat("data: "+strings.Repeat("x", 1024)+"\n", 1025)+"\n"), p); err == nil || p.errCode != 502 || p.finished {
		t.Fatal("unbounded event accepted")
	}
}

func TestReadSSECallbackStopAndErrors(t *testing.T) {
	for _, stop := range []error{io.EOF, errors.New("consumer failed")} {
		calls := 0
		body := io.MultiReader(strings.NewReader("\ufeffevent: result\r\ndata: one\r\ndata: two\r\n\r\n"), brokenReader{})
		err := ReadSSE(body, 1024, func(line string) error {
			if strings.HasPrefix(line, "data:") {
				calls++
				if line != "data: one\ntwo" {
					t.Fatal(line)
				}
				return stop
			}
			return nil
		})
		if calls != 1 {
			t.Fatalf("calls=%d", calls)
		}
		if stop == io.EOF && err != nil {
			t.Fatal(err)
		}
		if stop != io.EOF && err != stop {
			t.Fatalf("got %v want %v", err, stop)
		}
	}
}
