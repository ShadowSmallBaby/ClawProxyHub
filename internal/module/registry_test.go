package module

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDependenciesAndReverseCleanup(t *testing.T) {
	var calls []string
	makeModule := func(id string, deps ...string) Module {
		return &Func{Spec: Descriptor{ID: id, Requires: deps, Capabilities: []string{id}},
			RegisterFunc: func(Host) error { calls = append(calls, "register:"+id); return nil },
			StartFunc:    func(context.Context) error { calls = append(calls, "start:"+id); return nil },
			StopFunc:     func(context.Context) error { calls = append(calls, "stop:"+id); return nil },
		}
	}
	r, err := New(makeModule("tasks", "plugins"), makeModule("plugins", "storage"), makeModule("storage"))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	states := r.States()
	if !states[0].Available || !states[0].RestartRequired {
		t.Fatal(states)
	}
	states[0].Capabilities[0] = "mutated"
	if r.States()[0].Capabilities[0] == "mutated" {
		t.Fatal("mutable descriptor escaped")
	}
	if err := r.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"register:storage", "register:plugins", "register:tasks", "start:storage", "start:plugins", "start:tasks", "stop:tasks", "stop:plugins", "stop:storage"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls: %v", calls)
	}
	for _, state := range r.States() {
		if state.Available {
			t.Fatal("available after stop")
		}
	}
	if err := r.Start(context.Background(), nil); err == nil {
		t.Fatal("unexpected restart")
	}
}

func TestValidateBeforeRegistration(t *testing.T) {
	for _, tc := range []struct {
		name    string
		specs   []Descriptor
		message string
	}{
		{"duplicate", []Descriptor{{ID: "a"}, {ID: "a"}}, "duplicate"},
		{"missing", []Descriptor{{ID: "a", Requires: []string{"b"}}}, "a -> b"},
		{"cycle", []Descriptor{{ID: "a", Requires: []string{"b"}}, {ID: "b", Requires: []string{"a"}}}, "a -> b -> a"},
		{"capability", []Descriptor{{ID: "a", Capabilities: []string{"same"}}, {ID: "b", Capabilities: []string{"same"}}}, "provided by both"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var modules []Module
			for _, spec := range tc.specs {
				modules = append(modules, &Func{Spec: spec})
			}
			if _, err := New(modules...); err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error: %v", err)
			}
		})
	}
}

func TestFailureCleansPartialStateWithFreshContext(t *testing.T) {
	for _, phase := range []string{"register", "start"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("startup failed")
			cleanupFailure := errors.New("cleanup failed")
			var stopped []string
			a := &Func{Spec: Descriptor{ID: "a"}, StopFunc: func(ctx context.Context) error {
				if ctx.Err() != nil {
					t.Error("cleanup inherited cancelled context")
				}
				stopped = append(stopped, "a")
				return cleanupFailure
			}}
			b := &Func{Spec: Descriptor{ID: "b", Requires: []string{"a"}}, StopFunc: func(context.Context) error { stopped = append(stopped, "b"); return nil }}
			if phase == "register" {
				b.RegisterFunc = func(Host) error { cancel(); return failure }
			} else {
				b.StartFunc = func(context.Context) error { cancel(); return failure }
			}
			r, err := New(b, a)
			if err != nil {
				t.Fatal(err)
			}
			err = r.Start(ctx, nil)
			if !errors.Is(err, failure) || !errors.Is(err, cleanupFailure) {
				t.Fatalf("missing failure: %v", err)
			}
			if !reflect.DeepEqual(stopped, []string{"b", "a"}) {
				t.Fatal(stopped)
			}
			if err := r.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
