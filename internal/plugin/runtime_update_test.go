package plugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
)

func TestRuntimeUpdateSerializesPluginLifecycle(t *testing.T) {
	root, dir := pluginTestDir(t, "test")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"name":"test","runtime":"lua"}`), 0644); err != nil {
		t.Fatal(err)
	}
	connector := &pipeConnector{name: "test", protocol: sdk.ProtocolVersion}
	m := NewManager(root, nil, WithRuntime(NewServiceRuntime(connector)))
	if err := m.SetLuaRuntime(func() (string, error) { return "verified-service", nil }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.StopAll)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	old, err := m.Start(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	update, err := m.BeginLuaRuntimeUpdate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer update.Close()
	if connector.released.Load() != 1 {
		t.Fatal("runtime update did not stop its users")
	}
	entered, done := make(chan struct{}), make(chan error, 1)
	go func() {
		close(entered)
		_, err := m.Start(ctx, dir)
		done <- err
	}()
	<-entered
	select {
	case err := <-done:
		t.Fatalf("concurrent start escaped the runtime transaction: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	if err := m.SetLuaRuntime(func() (string, error) { return "new-runtime", nil }); err != nil {
		t.Fatal(err)
	}
	if err := update.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	update.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("queued plugin start was not released")
	}
	current, ok := m.Get("test")
	if !ok || current == old || connector.started.Load() != 2 {
		t.Fatal("concurrent start duplicated the restored session")
	}
}

type refusingClose struct{ Session }

func (refusingClose) Close() error { return errors.New("session is still running") }

func TestRuntimeUpdateAbortsWhenAPluginCannotStop(t *testing.T) {
	root, dir := pluginTestDir(t, "test")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"name":"test","runtime":"lua"}`), 0644); err != nil {
		t.Fatal(err)
	}
	connector := &pipeConnector{name: "test", protocol: sdk.ProtocolVersion}
	m := NewManager(root, nil, WithRuntime(NewServiceRuntime(connector)))
	if err := m.SetLuaRuntime(func() (string, error) { return "verified-service", nil }); err != nil {
		t.Fatal(err)
	}
	inst, err := m.Start(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	session := inst.session
	inst.session = refusingClose{session}
	t.Cleanup(func() { inst.session = session; m.StopAll() })
	if update, err := m.BeginLuaRuntimeUpdate(context.Background()); err == nil || update != nil {
		t.Fatal("runtime update ignored a failed stop")
	}
	if current, ok := m.Get("test"); !ok || current != inst || connector.started.Load() != 1 {
		t.Fatal("failed stop lost or duplicated the existing session")
	}
	if err := m.SetLuaRuntime(nil); err == nil {
		t.Fatal("runtime provider changed while its plugin was still running")
	}
}
