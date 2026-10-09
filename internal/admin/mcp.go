package admin

import (
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/mcpserver"
	"net/http"
)

// MCP 使用独立管理员密钥；配置和密钥管理仅接受完整 Web 管理会话。
func (s *Server) routeMCP(r authed) {
	if s.actions == nil {
		return
	}
	manager := mcpserver.NewManager(s.db, s.actions)
	r.mux.Handle("POST /admin/mcp", manager)
	r.h("GET /admin/mcp/config", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r) {
			return
		}
		state, err := manager.State(r.Context(), s.lookupUsername(r))
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "cannot read MCP configuration"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"config": state, "tools": s.actions.List(s.principal(r)), "endpoint": "/admin/mcp", "execution": "in-process"})
	})
	r.h("PUT /admin/mcp/config", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r) {
			return
		}
		var input struct {
			Enabled        bool     `json:"enabled"`
			AllowedActions []string `json:"allowed_actions"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 65536)
		if !readBody(w, r, &input) {
			return
		}
		if err := manager.Configure(r.Context(), s.lookupUsername(r), input.Enabled, input.AllowedActions); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	r.h("POST /admin/mcp/key", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r) {
			return
		}
		token, err := manager.Rotate(r.Context(), s.lookupUsername(r))
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "cannot issue MCP key"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]string{"key": token})
	})
	r.h("DELETE /admin/mcp/key", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r) {
			return
		}
		if err := manager.Revoke(r.Context(), s.lookupUsername(r)); err != nil {
			writeJSON(w, 500, map[string]string{"error": "cannot revoke MCP key"})
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
}
