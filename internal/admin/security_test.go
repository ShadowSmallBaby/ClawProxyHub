package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/router"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func TestDeleteLastRouteDoesNotGrantAll(t *testing.T) {
	db, _ := seedCascade(t)
	s := &Server{db: db}
	var k model.Key
	db.First(&k, "name = ?", "k1")
	db.Model(&k).Update("route_scope", "restricted")
	var rt model.Route
	db.First(&rt, "name = ?", "rt1")
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	req.SetPathValue("id", itoa(rt.ID))
	w := httptest.NewRecorder()
	s.deleteRoute(w, req)
	db.First(&k, k.ID)
	_, err := router.New(db).Resolve(&k, &pb.ChatRequest{Model: "rt2"})
	if err != router.ErrRouteForbidden {
		t.Fatalf("authorization widened: %v", err)
	}
}

func TestInvalidKeyBindingPreservesAuthorization(t *testing.T) {
	db, _ := seedCascade(t)
	s := &Server{db: db}
	var k model.Key
	db.First(&k, "name = ?", "k1")
	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"route_ids":[999999]}`))
	req.SetPathValue("id", itoa(k.ID))
	w := httptest.NewRecorder()
	s.bindKeyRoutes(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var n int64
	db.Model(&model.KeyRoute{}).Where("key_id = ?", k.ID).Count(&n)
	if n != 1 {
		t.Fatalf("bindings=%d", n)
	}
}

func TestRevealRequiresAdmin(t *testing.T) {
	w := httptest.NewRecorder()
	(&Server{}).revealKey(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d", w.Code)
	}
}
