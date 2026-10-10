package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/action"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

func TestScopedSessionsCannotEscapeActionsAndAreRevocable(t *testing.T) {
	db, _ := seedCascade(t)
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{Username: "scope-admin", Role: "admin"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	registry := action.New(nil)
	d := action.Descriptor{ID: "core.read", Owner: "core", Version: 1, Permission: "status.read", Effect: "read", TimeoutMS: 1000, Input: action.Schema{Type: "object"}, Output: action.Schema{Type: "object"}}
	registry.Register(d, func(context.Context, action.Principal, json.RawMessage) (any, error) { return map[string]any{}, nil })
	d.ID = "core.write"
	d.Permission = "workspace.write"
	d.Effect = "write"
	registry.Register(d, func(context.Context, action.Principal, json.RawMessage) (any, error) {
		t.Error("unauthorized handler ran")
		return map[string]any{}, nil
	})
	s := &Server{db: db, actions: registry}
	mux := http.NewServeMux()
	r := authed{mux, s}
	r.h("GET /admin/actions", s.listActions)
	r.h("POST /admin/actions/{id}", s.invokeAction)
	r.h("POST /admin/tokens/scoped", s.issueScopedToken)
	for _, path := range []string{"GET /admin/settings", "GET /admin/plugins", "POST /admin/password"} {
		r.h(path, func(w http.ResponseWriter, r *http.Request) { t.Error("scoped token escaped to generic API") })
	}
	token, err := s.signSession(user.Username, []string{"status.read"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+token)
		mux.ServeHTTP(w, req)
		return w
	}
	if w := call("GET", "/admin/actions"); w.Code != 200 || strings.Contains(w.Body.String(), "core.write") {
		t.Fatalf("scope list: %d %s", w.Code, w.Body.String())
	}
	if w := call("POST", "/admin/actions/core.read"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for _, pair := range [][2]string{{"POST", "/admin/actions/core.write"}, {"GET", "/admin/settings"}, {"GET", "/admin/plugins"}, {"POST", "/admin/password"}, {"POST", "/admin/tokens/scoped"}} {
		if w := call(pair[0], pair[1]); w.Code != 403 {
			t.Fatalf("escaped scope: %v %d", pair, w.Code)
		}
	}
	db.Model(&user).Update("auth_version", user.AuthVersion+1)
	if w := call("GET", "/admin/actions"); w.Code != 401 {
		t.Fatalf("revoked token accepted: %d", w.Code)
	}
}
