package admin

import (
	"context"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/action"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/database"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMCPManagementRequiresFullAdminSession(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	for _, role := range []string{"admin", "guest"} {
		if err = db.Create(&model.User{Username: role, Role: role}).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{db: db, actions: action.New(nil)}
	mux := http.NewServeMux()
	s.routeMCP(authed{mux, s})
	admin, _ := s.signSession("admin", nil, time.Hour)
	guest, _ := s.signSession("guest", nil, time.Hour)
	scoped, _ := s.signSession("admin", []string{"status.read"}, time.Hour)
	for _, tc := range []struct {
		token string
		code  int
	}{{admin, 200}, {guest, 403}, {scoped, 403}, {"cph_mcp_" + strings.Repeat("a", 64), 401}} {
		r := httptest.NewRequest("GET", "/admin/mcp/config", nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("management status %d, want %d", w.Code, tc.code)
		}
	}
	r := httptest.NewRequest("POST", "/admin/mcp", strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer "+admin)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("web session used as independent MCP key")
	}
}
