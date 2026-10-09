package action

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestPermissionSchemaCancellationAndUnregistration(t *testing.T) {
	var audits []Audit
	r := New(func(a Audit) error { audits = append(audits, a); return nil })
	started := make(chan struct{})
	d := Descriptor{ID: "example.run", Owner: "example", Version: 1, Permission: "workspace.execute", Effect: "execute", TimeoutMS: 1000, Input: Schema{Type: "object", Properties: map[string]Schema{"count": {Type: "integer"}}, Required: []string{"count"}}, Output: Schema{Type: "object"}}
	if err := r.Register(d, func(ctx context.Context, p Principal, b json.RawMessage) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	admin := Principal{Role: "admin", Subject: "admin"}
	limited := admin
	limited.Scopes = []string{"workspace.read"}
	if _, err := r.Invoke(t.Context(), limited, d.ID, []byte(`{"count":1}`)); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if len(r.List(limited)) != 0 {
		t.Fatal("listed unauthorized action")
	}
	for _, raw := range []string{`{}`, `{"count":1.5}`, `{"count":1,"unknown":true}`, `{"count":1} {}`} {
		if _, err := r.Invoke(t.Context(), admin, d.ID, []byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	done := make(chan error, 1)
	go func() { _, err := r.Invoke(context.Background(), admin, d.ID, []byte(` {"count":1} `)); done <- err }()
	<-started
	if err := r.RemoveOwner(t.Context(), "example"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if len(r.List(admin)) != 0 {
		t.Fatal("disabled action listed")
	}
	if _, err := r.Invoke(t.Context(), admin, d.ID, []byte(`{"count":1}`)); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if len(audits) != 6 {
		t.Fatalf("audit count %d", len(audits))
	}
}
func TestTimeoutAndOutputValidation(t *testing.T) {
	r := New(nil)
	d := Descriptor{ID: "core.read", Owner: "core", Version: 1, Permission: "workspace.read", Effect: "read", TimeoutMS: 5, Input: Schema{Type: "object"}, Output: Schema{Type: "boolean"}}
	if err := r.Register(d, func(ctx context.Context, p Principal, b json.RawMessage) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
			return true, nil
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Invoke(t.Context(), Principal{Role: "admin"}, d.ID, []byte(`{}`)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	_ = r.RemoveOwner(t.Context(), "core")
	_ = r.Register(d, func(context.Context, Principal, json.RawMessage) (any, error) { return "wrong type", nil })
	if _, err := r.Invoke(t.Context(), Principal{Role: "admin"}, d.ID, []byte(`{}`)); err == nil {
		t.Fatal("accepted invalid output")
	}
}
