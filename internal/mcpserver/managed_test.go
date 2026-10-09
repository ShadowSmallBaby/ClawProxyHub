package mcpserver

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/action"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/database"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

func TestManagedKeysAndLivePermissions(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "mcp.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	if err = db.Create(&model.User{Username: "admin", Role: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&model.User{Username: "guest", Role: "guest"}).Error; err != nil {
		t.Fatal(err)
	}
	registry := action.New(nil)
	started := make(chan struct{}, 1)
	for _, id := range []string{"core.read", "core.other", "core.wait"} {
		name := id
		err = registry.Register(action.Descriptor{ID: name, Owner: "core", Version: 1, Title: name, Permission: "status.read", Effect: "read", TimeoutMS: 30000, Input: action.Schema{Type: "object"}, Output: action.Schema{Type: "object"}}, func(ctx context.Context, _ action.Principal, _ json.RawMessage) (any, error) {
			if name == "core.wait" {
				started <- struct{}{}
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return map[string]any{}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	manager := NewManager(db, registry)
	key, err := manager.Rotate(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	db.Table("mcp_credentials").Select("token_hash").Where("username=?", "admin").Scan(&stored)
	if stored == key || len(stored) != 64 {
		t.Fatal("MCP key must be stored only as a digest")
	}
	request := func(token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "http://localhost/admin/mcp", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		result := httptest.NewRecorder()
		manager.ServeHTTP(result, req)
		return result
	}
	list := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	if request(key, list).Code != 503 {
		t.Fatal("MCP must start disabled")
	}
	if _, err = manager.Rotate(ctx, "guest"); err == nil {
		t.Fatal("guest received MCP key")
	}
	if err = manager.Configure(ctx, "admin", true, []string{"core.read"}); err != nil {
		t.Fatal(err)
	}
	if got := request(key, list); got.Code != 200 || !strings.Contains(got.Body.String(), "core.read") || strings.Contains(got.Body.String(), "core.other") {
		t.Fatal(got.Body.String())
	}
	if got := request(key, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"core.other"}}`); !strings.Contains(got.Body.String(), "tool not exposed") {
		t.Fatal("same permission does not grant another action")
	}
	if err = manager.Configure(ctx, "admin", true, []string{"core.wait"}); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		request(key, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"core.wait"}}`)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("tool did not start")
	}
	if err = manager.Configure(ctx, "admin", true, []string{"core.other"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("permission change did not cancel active tool")
	}
	next, err := manager.Rotate(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if request(key, list).Code != 401 || request(next, list).Code != 200 {
		t.Fatal("key rotation did not replace the old key")
	}
	var count int64
	db.Table("mcp_credentials").Where("username=?", "admin").Count(&count)
	if count != 1 {
		t.Fatal("more than one key per user")
	}
	// 重新创建管理器验证启停、密钥和权限不会随着核心重启丢失。
	manager = NewManager(db, registry)
	if got := request(next, list); got.Code != 200 || !strings.Contains(got.Body.String(), "core.other") {
		t.Fatal("MCP state did not persist")
	}
	if err = manager.Revoke(ctx, "admin"); err != nil {
		t.Fatal(err)
	}
	if request(next, list).Code != 401 {
		t.Fatal("revoked key accepted")
	}
	next, err = manager.Rotate(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	db.Model(&model.User{}).Where("username=?", "admin").Update("role", "guest")
	if request(next, list).Code != 401 {
		t.Fatal("demoted user kept MCP access")
	}
}
