package admin

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/database"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

func TestDeviceSetupAndLegacyClaim(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "legacy"}[legacy], func(t *testing.T) {
			db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "device.db"))
			if err != nil {
				t.Fatal(err)
			}
			sql, _ := db.DB()
			defer sql.Close()
			s := &Server{db: db}
			oldToken := ""
			if legacy {
				if !s.createUser("device-owner", strings.Repeat("a", 64)) {
					t.Fatal("seed")
				}
				oldToken, err = s.DeviceSession()
				if err != nil {
					t.Fatal(err)
				}
			}
			WithDeviceSetup()(s)
			token, err := s.DeviceSession()
			if err != nil || token != "" || s.initialized() {
				t.Fatalf("setup bypass: %q %v", token, err)
			}
			if legacy {
				if _, err = s.parseJWT(oldToken); err == nil {
					t.Fatal("legacy session still usable before claim")
				}
			}
			status := httptest.NewRecorder()
			s.setupStatus(status, httptest.NewRequest("GET", "/admin/setup-status", nil))
			if !strings.Contains(status.Body.String(), `"initialized":false`) {
				t.Fatal(status.Body.String())
			}
			request := func() *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				s.setup(w, httptest.NewRequest("POST", "/admin/setup", strings.NewReader(`{"username":"owner","password":"my-secret-password"}`)))
				return w
			}
			if w := request(); w.Code != 200 {
				t.Fatalf("setup: %d %s", w.Code, w.Body.String())
			}
			if !s.verifyPassword("owner", "my-secret-password") {
				t.Fatal("user credentials not set")
			}
			if w := request(); w.Code != 409 {
				t.Fatal("setup allowed twice")
			}
			var users []model.User
			db.Find(&users)
			if len(users) != 1 || users[0].ID != 1 {
				t.Fatal("claim replaced user identity")
			}
			if token, err = s.DeviceSession(); err != nil || token == "" {
				t.Fatal("native session unavailable", err)
			}
			if legacy {
				if _, err = s.parseJWT(oldToken); err == nil {
					t.Fatal("old session not revoked")
				}
			}
			restarted := &Server{db: db, deviceSetup: true}
			if !restarted.initialized() {
				t.Fatal("setup not persistent")
			}
		})
	}
}

func TestDesktopDeviceOwnerDoesNotReopenSetup(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "desktop.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	s := &Server{db: db}
	if !s.createUser("device-owner", "ordinary-password") || !s.initialized() {
		t.Fatal("desktop account altered")
	}
}
