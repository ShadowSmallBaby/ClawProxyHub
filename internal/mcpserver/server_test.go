package mcpserver

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/action"
)

func send(s *Server, p action.Principal, session, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.HTTP(w, r, p, session)
	return w
}
func TestVisibilityCancellationAndRemoval(t *testing.T) {
	registry := action.New(nil)
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	d := action.Descriptor{ID: "demo.wait", Owner: "demo", Version: 1, Permission: "tasks.execute", Effect: "execute", TimeoutMS: 10000, Input: action.Schema{Type: "object"}, Output: action.Schema{Type: "object"}}
	if err := registry.Register(d, func(ctx context.Context, _ action.Principal, _ json.RawMessage) (any, error) {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		return nil, ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	s := New(registry, []string{"demo.wait"})
	admin := action.Principal{Role: "admin"}
	reader := action.Principal{Role: "admin", Scopes: []string{"tasks.read"}}
	list := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	if strings.Contains(send(s, reader, "reader", list).Body.String(), "demo.wait") {
		t.Fatal("tool escaped scope")
	}
	if !strings.Contains(send(s, admin, "a", list).Body.String(), "demo.wait") {
		t.Fatal("allowed tool missing")
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- send(s, admin, "a", `{"jsonrpc":"2.0","id":"work","method":"tools/call","params":{"name":"demo.wait","arguments":{}}}`)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("call did not start")
	}
	cancellation := `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"work"}}`
	send(s, admin, "b", cancellation)
	select {
	case <-cancelled:
		t.Fatal("another session cancelled this call")
	case <-time.After(25 * time.Millisecond):
	}
	send(s, admin, "a", cancellation)
	select {
	case result := <-done:
		if !strings.Contains(result.Body.String(), `"isError":true`) {
			t.Fatal(result.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation was not propagated")
	}
	if err := registry.RemoveOwner(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(send(s, admin, "a", list).Body.String(), "demo.wait") {
		t.Fatal("disabled extension leaked tool")
	}
	if !strings.Contains(send(s, admin, "a", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"demo.wait"}}`).Body.String(), "unavailable") {
		t.Fatal("disabled tool callable")
	}
}

func TestExplicitAllowlistAndProtocol(t *testing.T) {
	s := New(action.New(nil), nil)
	p := action.Principal{Role: "admin"}
	for _, body := range []string{`[]`, `{"jsonrpc":"2.0","id":{},"method":"ping"}`, `broken`} {
		w := send(s, p, "a", body)
		if !strings.Contains(w.Body.String(), `"error"`) {
			t.Fatal(w.Body.String())
		}
	}
	w := send(s, p, "a", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`)
	if !strings.Contains(w.Body.String(), `"listChanged":false`) {
		t.Fatal(w.Body.String())
	}
}
