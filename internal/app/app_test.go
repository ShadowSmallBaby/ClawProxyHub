package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/config"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/module"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CPH_ADMIN_USERNAME", "admin")
	t.Setenv("CPH_ADMIN_PASSWORD", "p1-test-password")
	t.Setenv("CPH_SEED_API_KEY", "")
	t.Setenv("CPH_SECRET_KEY", "")
	return &config.Config{Addr: "127.0.0.1:0", DataDir: dir, DatabaseDSN: filepath.Join(dir, "cph.db"), PluginDir: filepath.Join(dir, "plugins")}
}

func TestWebOptionPreservesBackendCapabilities(t *testing.T) {
	for _, test := range []struct {
		name string
		web  bool
	}{{"embedded-web", true}, {"native-host", false}} {
		t.Run(test.name, func(t *testing.T) {
			options := []Option{WithWeb(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("web")) }))}
			if !test.web {
				options = append(options, WithoutWeb())
			}
			a, err := New(testConfig(t), options...)
			if err != nil {
				t.Fatal(err)
			}
			if err := a.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := a.Shutdown(ctx); err != nil {
					t.Error(err)
				}
			})
			client := &http.Client{Timeout: 5 * time.Second}
			base := "http://" + a.Address()
			request := func(method, path, body, token string) (int, []byte) {
				t.Helper()
				req, err := http.NewRequest(method, base+path, strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				if token != "" {
					req.Header.Set("Authorization", "Bearer "+token)
				}
				resp, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				data, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				return resp.StatusCode, data
			}
			if code, _ := request("GET", "/health", "", ""); code != 200 {
				t.Fatalf("health=%d", code)
			}
			if code, _ := request("GET", "/admin/capabilities", "", ""); code != 401 {
				t.Fatalf("capabilities leaked without auth: %d", code)
			}
			code, data := request("POST", "/admin/login", `{"username":"admin","password":"p1-test-password"}`, "")
			if code != 200 {
				t.Fatalf("login %d: %s", code, data)
			}
			var login struct {
				Token string `json:"token"`
			}
			if err := json.Unmarshal(data, &login); err != nil {
				t.Fatal(err)
			}
			if login.Token == "" {
				t.Fatal("missing login token")
			}
			code, data = request("GET", "/admin/capabilities", "", login.Token)
			if code != 200 {
				t.Fatalf("capabilities=%d", code)
			}
			var caps struct {
				Profile string         `json:"profile"`
				Modules []module.State `json:"modules"`
			}
			if err := json.Unmarshal(data, &caps); err != nil {
				t.Fatal(err)
			}
			if caps.Profile != "full" {
				t.Fatal(caps.Profile)
			}
			gateway, web := false, false
			for _, state := range caps.Modules {
				if !state.Available {
					t.Fatalf("module unavailable: %s", state.ID)
				}
				gateway = gateway || state.ID == "gateway"
				web = web || state.ID == "web"
			}
			if !gateway {
				t.Fatal("incorrect gateway capability")
			}
			if web != test.web {
				t.Fatalf("web module=%v, want %v", web, test.web)
			}
			webCode := http.StatusOK
			if !test.web {
				webCode = http.StatusNotFound
			}
			if code, _ := request("GET", "/", "", ""); code != webCode {
				t.Fatalf("web=%d, want %d", code, webCode)
			}
			for _, path := range []string{"/admin/task-rules", "/admin/accounts", "/admin/run-logs"} {
				if code, _ := request("GET", path, "", login.Token); code != 200 {
					t.Fatalf("%s: %d", path, code)
				}
			}
			for _, path := range []string{"/admin/keys", "/admin/routes", "/admin/logs"} {
				want := 200
				if code, _ := request("GET", path, "", login.Token); code != want {
					t.Fatalf("%s: %d != %d", path, code, want)
				}
			}
			want := 401
			if code, _ := request("POST", "/v1/chat/completions", `{}`, ""); code != want {
				t.Fatalf("gateway: %d != %d", code, want)
			}
			code, data = request("GET", "/admin/me", "", login.Token)
			if code != 200 {
				t.Fatal(code)
			}
			var me struct {
				Menus []string `json:"menus"`
			}
			if err := json.Unmarshal(data, &me); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"keys", "routes"} {
				found := false
				for _, menu := range me.Menus {
					found = found || menu == want
				}
				if !found {
					t.Fatalf("missing menu %s", want)
				}
			}
			if err := a.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			db, _ := a.db.DB()
			if err := db.Ping(); err == nil {
				t.Fatal("database remained open")
			}
		})
	}
}

func TestListenFailureCleansStartedServices(t *testing.T) {
	cfg := testConfig(t)
	cfg.Addr = "invalid-listen-address"
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Start(context.Background()); err == nil {
		t.Fatal("expected listener failure")
	}
	for _, done := range []<-chan struct{}{a.refreshDone, a.retentionDone} {
		select {
		case <-done:
		default:
			t.Fatal("background service survived startup failure")
		}
	}
	db, _ := a.db.DB()
	if err := db.Ping(); err == nil {
		t.Fatal("database leaked")
	}
	for _, state := range a.States() {
		if state.Available {
			t.Fatal("failed module still available")
		}
	}
}

func TestProfileDefaultAndValidation(t *testing.T) {
	cfg := testConfig(t)
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if a.cfg.Profile != "full" {
		t.Fatal(a.cfg.Profile)
	}
	for _, profile := range []string{"unknown", "task"} {
		cfg.Profile = profile
		if _, err := New(cfg); err == nil {
			t.Fatalf("accepted removed/unknown profile %s", profile)
		}
	}
}
