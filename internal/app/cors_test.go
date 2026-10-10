package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminCORS(t *testing.T) {
	const origin = "https://client.example"
	handler := adminCORS([]string{origin}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	for _, test := range []struct {
		name, path, origin, method, requestedMethod, headers string
		status                                               int
		cors                                                 bool
	}{
		{"preflight", "/admin/login", origin, "OPTIONS", "POST", "authorization,content-type", 204, true},
		{"auth remains required", "/admin/capabilities", origin, "GET", "", "", 401, true},
		{"asset", "/assets/plugins/test/icon", origin, "GET", "", "", 401, true},
		{"unknown origin", "/admin/login", "https://other.example", "OPTIONS", "POST", "", 403, false},
		{"null origin", "/admin/login", "null", "OPTIONS", "POST", "", 403, false},
		{"unapproved response", "/admin/me", "https://other.example", "GET", "", "", 401, false},
		{"method", "/admin/me", origin, "OPTIONS", "PATCH", "", 403, true},
		{"header", "/admin/me", origin, "OPTIONS", "GET", "x-admin-token", 403, true},
		{"gateway unchanged", "/v1/models", origin, "GET", "", "", 401, false},
		{"same origin", "/admin/me", "", "GET", "", "", 401, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(test.method, test.path, nil)
			r.Header.Set("Origin", test.origin)
			r.Header.Set("Access-Control-Request-Method", test.requestedMethod)
			r.Header.Set("Access-Control-Request-Headers", test.headers)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("status %d, want %d", w.Code, test.status)
			}
			if got := w.Header().Get("Access-Control-Allow-Origin"); (got != "") != test.cors || (got != "" && got != test.origin) {
				t.Fatalf("origin %q", got)
			}
			if w.Header().Get("Access-Control-Allow-Credentials") != "" {
				t.Fatal("unexpected cookie credentials")
			}
			if test.cors && w.Header().Get("Access-Control-Expose-Headers") != "Content-Disposition" {
				t.Fatal("download filename not exposed")
			}
		})
	}
}
