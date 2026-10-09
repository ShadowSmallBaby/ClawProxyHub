package extension

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"
)

func TestRPCBidirectionalConcurrencyAndCancellation(t *testing.T) {
	a, b := net.Pipe()
	cancelled := make(chan struct{})
	handler := func(ctx context.Context, method string, raw json.RawMessage) (any, error) {
		if method == "wait" {
			<-ctx.Done()
			close(cancelled)
			return nil, ctx.Err()
		}
		return raw, nil
	}
	left := NewPeer(a, a, handler)
	right := NewPeer(b, b, handler)
	defer left.Close()
	defer right.Close()
	var group sync.WaitGroup
	for i := 0; i < 20; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			peer := left
			if i%2 == 0 {
				peer = right
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			raw, err := peer.Call(ctx, "echo", i)
			if err != nil {
				t.Error(err)
				return
			}
			var value int
			if json.Unmarshal(raw, &value) != nil || value != i {
				t.Error("incorrect RPC response")
			}
		}(i)
	}
	group.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := left.Call(ctx, "wait", nil); err == nil {
		t.Fatal("cancelled RPC succeeded")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("remote handler was not cancelled")
	}
}
