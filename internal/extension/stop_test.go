package extension

import (
	"context"
	"errors"
	"testing"
)

func TestInterruptedDeactivationCanBeRetried(t *testing.T) {
	f := newFixture(t)
	m := New(t.TempDir(), "1.5.2", f.trust)
	ctx := context.Background()
	m.Load(ctx)
	attempts := 0
	m.SetLifecycle(func(context.Context, State) error { return nil }, func(context.Context, string) error {
		attempts++
		if attempts == 1 {
			return context.DeadlineExceeded
		}
		return nil
	})
	if _, e := m.Install(ctx, f.pack("editor", "1.0.0", nil, nil), []string{"workspace.read"}); e != nil {
		t.Fatal(e)
	}
	if e := m.SetEnabled(ctx, "editor", false); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	if state, _ := m.State("editor"); state.Available || state.Status != "stopping" {
		t.Fatal("interrupted deactivation reported available")
	}
	if e := m.SetEnabled(ctx, "editor", false); e != nil {
		t.Fatal(e)
	}
	if attempts != 2 {
		t.Fatal("cleanup was not retried")
	}
	if e := m.SetEnabled(ctx, "editor", true); e != nil {
		t.Fatal(e)
	}
}
