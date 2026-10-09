package luaeditor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extstore"
	ext "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

type draftHarness struct {
	peer       *ext.Peer
	store      *extstore.Store
	session    *extstore.Session
	failCommit atomic.Bool
	done       chan error
}

func newDraftHarness(t *testing.T, path string) *draftHarness {
	t.Helper()
	raw, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Storage ext.StorageSchema `json:"storage"`
		Actions []ext.Action      `json:"actions"`
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	store, err := extstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Bind(context.Background(), "lua-editor", "test", strings.Repeat("a", 64), manifest.Storage)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	h := &draftHarness{store: store, session: session, done: make(chan error, 1)}
	server, client := net.Pipe()
	go func() { h.done <- ext.ServeIO(context.Background(), server, server, Service()) }()
	h.peer = ext.NewPeer(client, client, func(ctx context.Context, method string, raw json.RawMessage) (any, error) {
		var call ext.StorageCall
		if err := ext.DecodeStrict(raw, &call); err != nil {
			return nil, err
		}
		if method != "storage" || call.Capability != "test-invocation" {
			return nil, fmt.Errorf("invalid storage capability")
		}
		for _, request := range call.Requests {
			if request.Op == "insert" && request.Table == "drafts" && h.failCommit.CompareAndSwap(true, false) {
				return nil, fmt.Errorf("simulated interruption before draft commit")
			}
		}
		return session.Execute(ctx, call.Requests, []string{"storage.read", "storage.write"})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err = h.peer.Call(ctx, "initialize", ext.Initialize{Protocol: ext.BackendProtocol, ID: "lua-editor", Version: "0.2.0", PackageHash: strings.Repeat("a", 64), Actions: manifest.Actions}); err != nil {
		h.close()
		t.Fatal(err)
	}
	return h
}
func (h *draftHarness) close() {
	h.peer.Close()
	<-h.done
	h.session.Close()
	h.store.Close()
}
func (h *draftHarness) call(action string, input any) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := h.peer.Call(ctx, "invoke", ext.Invocation{Action: action, Capability: "test-invocation", Input: rawValue(input)})
	var out map[string]any
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return out, err
}

func TestDraftSurvivesRestartAndRejectsStaleWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drafts.db")
	h := newDraftHarness(t, path)
	defer func() {
		if h != nil {
			h.close()
		}
	}()
	// 超过单行和单次分块查询的容量，中文可跨字节块边界。
	content := strings.Repeat("-- 草稿 α\n", 75000)
	input := draftInput{Name: "example", Content: content, BaseSHA256: hashSource("return {}")}
	saved, err := h.call("draft-save", input)
	if err != nil {
		t.Fatal(err)
	}
	input.Revision = saved["revision"].(string)
	read := func(want string) {
		t.Helper()
		result, err := h.call("draft-read", draftKey{Name: "example"})
		if err != nil || result["found"] != true {
			t.Fatalf("read: %v, %v", result, err)
		}
		if draft := result["draft"].(map[string]any); draft["content"] != want || draft["base_sha256"] != input.BaseSHA256 {
			t.Fatal("draft changed or was truncated")
		}
	}
	read(content)
	h.failCommit.Store(true)
	input.Content = "local PLUGIN_NAME = 'example'\nfunction unfinished("
	if _, err = h.call("draft-save", input); err == nil {
		t.Fatal("interrupted commit succeeded")
	}
	read(content)
	next, err := h.call("draft-save", input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.call("draft-save", input); err == nil {
		t.Fatal("stale draft overwrite accepted")
	}
	if _, err = h.call("draft-delete", draftVersion{Name: "example", Revision: input.Revision}); err == nil {
		t.Fatal("stale draft deletion accepted")
	}
	h.close()
	h = nil
	h = newDraftHarness(t, path)
	read(input.Content)
	if _, err = h.call("draft-delete", draftVersion{Name: "example", Revision: next["revision"].(string)}); err != nil {
		t.Fatal(err)
	}
	result, err := h.call("draft-read", draftKey{Name: "example"})
	if err != nil || result["found"] != false {
		t.Fatalf("deleted draft remains: %v, %v", result, err)
	}
	chunks, err := h.session.Execute(context.Background(), []ext.StorageRequest{{Op: "query", Table: "draft_chunks"}}, []string{"storage.read"})
	if err != nil || len(chunks[0].Rows) != 0 {
		t.Fatal("draft chunks were not cleaned:", err)
	}
}

func TestDraftNamesAndInputCannotEscapeDeclaredStorage(t *testing.T) {
	h := newDraftHarness(t, filepath.Join(t.TempDir(), "drafts.db"))
	defer h.close()
	for _, input := range []any{map[string]any{"name": "../../other"}, map[string]any{"name": "example", "table": "other"}} {
		if _, err := h.call("draft-read", input); err == nil {
			t.Fatal("untrusted draft input accepted")
		}
	}
	for _, name := range []string{"", "new"} {
		if _, err := h.call("draft-save", draftInput{Name: name, Content: name, BaseSHA256: hashSource("")}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := h.call("draft-read", draftKey{Name: "new"})
	if err != nil || result["draft"].(map[string]any)["content"] != "new" {
		t.Fatal("new plugin draft collided with plugin named new:", err)
	}
}
