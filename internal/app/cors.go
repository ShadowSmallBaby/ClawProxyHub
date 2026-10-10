package app

import (
	"net/http"
	"strings"
)

// adminCORS 仅开放配置过的客户端来源，Bearer 鉴权仍由管理处理器执行。
func adminCORS(origins []string, next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowed[origin] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/admin/") && !strings.HasPrefix(r.URL.Path, "/assets/plugins/") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if !allowed[origin] {
			// 同源和 CLI 不依赖 CORS；跨域预检直接拒绝。
			if r.Method == http.MethodOptions && origin != "" {
				http.Error(w, "origin is not allowed", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			switch r.Header.Get("Access-Control-Request-Method") {
			case "GET", "POST", "PUT", "DELETE":
			default:
				http.Error(w, "method is not allowed", http.StatusForbidden)
				return
			}
			for _, header := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
				switch strings.ToLower(strings.TrimSpace(header)) {
				case "", "authorization", "content-type":
				default:
					http.Error(w, "header is not allowed", http.StatusForbidden)
					return
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
