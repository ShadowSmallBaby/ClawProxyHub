package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/task"
)

func TestSettingsTimezone(t *testing.T) {
	db, _ := seedCascade(t)
	store := setting.New(db)
	engine := task.NewEngine(db, t.TempDir(), nil, nil, store)
	defer engine.Stop()
	s := &Server{db: db, settings: store, engine: engine}
	for _, zone := range []string{"", "Local", "Asia/Beijing", "../etc/passwd", "UTC+8"} {
		body, _ := json.Marshal(map[string]interface{}{"timezone": zone, "task_daily_jitter": 0})
		w := httptest.NewRecorder()
		s.putSettings(w, httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("zone=%q status=%d", zone, w.Code)
		}
		if store.Timezone() != setting.DefaultTimezone || store.DailyJitter().Minutes() != setting.DefaultTaskDailyJitter {
			t.Fatal("invalid save changed settings")
		}
	}
	w := httptest.NewRecorder()
	s.putSettings(w, httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"timezone":"Europe/London"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	s.getSettings(w, httptest.NewRequest(http.MethodGet, "/", nil))
	var body struct{ Settings struct{ Timezone string } }
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Settings.Timezone != "Europe/London" {
		t.Fatalf("timezone=%q", body.Settings.Timezone)
	}
}
