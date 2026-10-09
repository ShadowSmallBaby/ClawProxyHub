package web

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedProfile(t *testing.T) {
	data, err := fs.ReadFile(distFS, distDir+"/profile.json")
	if err != nil {
		if _, statErr := fs.Stat(distFS, distDir+"/index.html"); statErr != nil {
			t.Skip("frontend has not been built")
		}
		t.Fatal(err)
	}
	var profile struct {
		Profile string   `json:"profile"`
		Modules []string `json:"modules"`
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatal(err)
	}
	if profile.Profile != "web-full" {
		t.Fatalf("embedded %s instead of web-full", profile.Profile)
	}
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `id="app"`) {
		t.Fatalf("SPA fallback: %d", w.Code)
	}
}
