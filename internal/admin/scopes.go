package admin

import (
	"encoding/json"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
	"net/http"
	"time"
)

// issueScopedToken 仅完整管理员会话可委派，不能经受限令牌扩大权限。
func (s *Server) issueScopedToken(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var in struct {
		Scopes     []string `json:"scopes"`
		TTLSeconds int      `json:"ttl_seconds"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid token request"})
		return
	}
	if len(in.Scopes) == 0 || len(in.Scopes) > 64 || in.TTLSeconds < 60 || in.TTLSeconds > 86400 {
		writeJSON(w, 400, map[string]string{"error": "provide 1..64 scopes and TTL 60..86400 seconds"})
		return
	}
	known := map[string]bool{}
	if s.actions != nil {
		for _, d := range s.actions.List(s.principal(r)) {
			known[d.Permission] = true
		}
	}
	seen := map[string]bool{}
	for _, scope := range in.Scopes {
		if !spec.ValidID(scope) || !known[scope] || seen[scope] {
			writeJSON(w, 400, map[string]string{"error": "unknown or duplicate action scope"})
			return
		}
		seen[scope] = true
	}
	token, err := s.signSession(s.lookupUsername(r), in.Scopes, time.Duration(in.TTLSeconds)*time.Second)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "cannot issue session"})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"token": token, "scopes": in.Scopes, "expires_in": in.TTLSeconds})
}
