package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCLIAndStdioUseExistingBackend(t *testing.T) {
	t.Setenv("CPH_TOKEN", "test-only-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-only-token" {
			t.Error("missing bearer")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/admin/mcp" {
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`))
			return
		}
		if r.URL.Path != "/admin/actions/core.status" {
			t.Error(r.URL.Path)
		}
		w.Write([]byte(`{"profile":"full"}`))
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := run(context.Background(), []string{"--url", server.URL, "status"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != `{"profile":"full"}` {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := run(context.Background(), []string{"--url", server.URL, "mcp"}, strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}\n"), &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}` {
		t.Fatalf("stdio contains non-protocol output: %s", out.String())
	}
}

func TestStdioCancellationReachesBackendWhenWorkSlotsAreFull(t *testing.T) {
	t.Setenv("CPH_TOKEN", "test-only-token")
	started := make(chan struct{}, 32)
	cancelled := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage
			Method string
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Method == "notifications/cancelled" {
			close(cancelled)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		started <- struct{}{}
		select {
		case <-cancelled:
		case <-ctx.Done():
			t.Error("busy bridge did not forward cancellation")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{}}`, request.ID)
	}))
	defer server.Close()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	done := make(chan error, 1)
	go func() { done <- run(ctx, []string{"--url", server.URL, "mcp"}, reader, io.Discard) }()
	for i := 0; i < 32; i++ {
		fmt.Fprintf(writer, "{\"jsonrpc\":\"2.0\",\"id\":%d,\"method\":\"tools/call\"}\n", i)
	}
	for i := 0; i < 32; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("requests did not fill work slots")
		}
	}
	fmt.Fprintln(writer, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":0}}`)
	writer.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
