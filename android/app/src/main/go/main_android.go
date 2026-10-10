package main

/*
#include <stdlib.h>
#include "platform.h"
*/
import "C"
import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/app"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/config"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extension"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extservice"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/platform/androidconn"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/plugin"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

var lock sync.Mutex
var core *app.App
var coreDir string

func verifyRuntime(ctx context.Context, path string) error {
	return plugin.ProbeLuaRuntime(ctx, plugin.NewServiceRuntime(connector{runtimeArchive: path}))
}

func systemComponents() []extension.SystemComponent {
	value := C.CPHPlatformComponents()
	if value == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(value))
	var components []extension.SystemComponent
	if json.Unmarshal([]byte(C.GoString(value)), &components) != nil {
		return nil
	}
	return components
}

var available sync.Map
var jobs sync.Map
var jobSerial atomic.Int64

type systemJob struct {
	ctx    context.Context
	cancel context.CancelFunc
}

//export CPHCoreNewJob
func CPHCoreNewJob() C.longlong {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	id := jobSerial.Add(1)
	jobs.Store(id, systemJob{ctx, cancel})
	return C.longlong(id)
}

func response(v any, err error) *C.char {
	if err != nil {
		v = map[string]any{"error": err.Error()}
	}
	data, _ := json.Marshal(v)
	return C.CString(string(data))
}
func discover(dir string) error {
	raw := C.CPHPlatformDiscover()
	if raw == nil {
		return fmt.Errorf("system discovery unavailable")
	}
	defer C.free(unsafe.Pointer(raw))
	var manifests []plugin.PackageManifest
	if err := json.Unmarshal([]byte(C.GoString(raw)), &manifests); err != nil {
		return err
	}
	next := map[string]bool{}
	for _, m := range manifests {
		if m.Android == nil || !validName(m.Name) {
			return fmt.Errorf("invalid system package identity")
		}
		next[m.Name] = true
		path := filepath.Join(dir, "plugins", "system", m.Name)
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
		data, _ := json.Marshal(m)
		if err := os.WriteFile(filepath.Join(path, "manifest.json"), data, 0600); err != nil {
			return err
		}
		available.Store(m.Name, true)
	}
	available.Range(func(k, v any) bool {
		if !next[k.(string)] {
			available.Delete(k)
		}
		return true
	})
	return nil
}
func validName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

type connector struct{ runtimeArchive string }

func (connector) Available(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	var m plugin.PackageManifest
	if err == nil && json.Unmarshal(data, &m) == nil && m.Runtime == "lua" {
		return C.CPHPlatformHasLua() != 0
	}
	_, ok := available.Load(filepath.Base(dir))
	return ok
}
func (c connector) Connect(ctx context.Context, dir string) (*plugin.ServiceBinding, error) {
	name := C.CString(dir)
	defer C.free(unsafe.Pointer(name))
	var b C.CPHBinding
	if c.runtimeArchive == "" {
		b = C.CPHPlatformConnect(name)
	} else {
		archive := C.CString(c.runtimeArchive)
		defer C.free(unsafe.Pointer(archive))
		b = C.CPHPlatformConnectRuntime(name, archive)
	}
	if b.error != nil {
		defer C.free(unsafe.Pointer(b.error))
	}
	if b.id == 0 {
		if b.error != nil {
			return nil, fmt.Errorf("Android worker connection failed: %s", C.GoString(b.error))
		}
		return nil, fmt.Errorf("Android private worker connection failed")
	}
	requests, e1 := androidconn.Take(int(b.requests))
	callbacks, e2 := androidconn.Take(int(b.callbacks))
	if e1 != nil || e2 != nil || ctx.Err() != nil {
		if requests != nil {
			requests.Close()
		}
		if callbacks != nil {
			callbacks.Close()
		}
		C.CPHPlatformRelease(b.id)
		return nil, fmt.Errorf("invalid or cancelled Android connection")
	}
	return &plugin.ServiceBinding{Requests: requests, Callbacks: callbacks, Protocol: 2, Release: func() error { C.CPHPlatformRelease(b.id); return nil }}, nil
}

// connectExtension 只接管已验签入口的 socket，停止时同时撤销 Binder 会话。
func connectExtension(ctx context.Context, path, digest string, minSDK int) (*extservice.Connection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	library, hash := C.CString(path), C.CString(digest)
	defer C.free(unsafe.Pointer(library))
	defer C.free(unsafe.Pointer(hash))
	binding := C.CPHPlatformConnectExtension(library, hash, C.int(minSDK))
	if binding.error != nil {
		defer C.free(unsafe.Pointer(binding.error))
	}
	if binding.id == 0 {
		if binding.error != nil {
			return nil, fmt.Errorf("Android extension connection failed: %s", C.GoString(binding.error))
		}
		return nil, fmt.Errorf("Android extension connection failed")
	}
	connection, err := androidconn.Take(int(binding.socket))
	if err != nil || ctx.Err() != nil {
		if connection != nil {
			_ = connection.Close()
		}
		C.CPHPlatformReleaseExtension(binding.id)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	var once sync.Once
	return &extservice.Connection{Reader: connection, Writer: connection, Close: func() error {
		once.Do(func() {
			_ = connection.Close()
			C.CPHPlatformReleaseExtension(binding.id)
		})
		return nil
	}}, nil
}

//export CPHCoreStart
func CPHCoreStart(directory *C.char) *C.char {
	lock.Lock()
	defer lock.Unlock()
	dir := C.GoString(directory)
	if core == nil {
		temporary := filepath.Join(dir, "tmp")
		if err := os.MkdirAll(temporary, 0700); err != nil {
			return response(nil, err)
		}
		if err := os.Setenv("TMPDIR", temporary); err != nil {
			return response(nil, err)
		}
		if err := discover(dir); err != nil {
			return response(nil, err)
		}
		value := C.CPHPlatformExtensionTrust()
		if value == nil {
			return response(nil, fmt.Errorf("Android package trust unavailable"))
		}
		var trust spec.TrustStore
		err := json.Unmarshal([]byte(C.GoString(value)), &trust)
		C.free(unsafe.Pointer(value))
		if err != nil {
			return response(nil, err)
		}
		a, err := app.New(&config.Config{AdminOrigins: []string{"https://appassets.androidplatform.net"}, Addr: "127.0.0.1:0", DataDir: dir, DatabaseDSN: filepath.Join(dir, "cph.db"), PluginDir: filepath.Join(dir, "plugins"), PackageDirs: []string{filepath.Join(dir, "packages"), filepath.Join(dir, "packages", "bundled")}}, app.WithoutWeb(), app.WithDeviceSetup(), app.WithExtensionTrust(trust), app.WithExtensionConnector(connectExtension), app.WithRuntimeValidator(verifyRuntime), app.WithNativeRuntimeManagement(), app.WithSystemComponents(systemComponents), app.WithExternalScheduler(), app.WithPluginRuntime(plugin.NewServiceRuntime(connector{})))
		if err != nil {
			return response(nil, err)
		}
		if err = a.Start(context.Background()); err != nil {
			return response(nil, err)
		}
		core = a
		coreDir = dir
	}
	token, err := core.DeviceSession()
	return response(map[string]any{"address": "http://" + core.Address(), "token": token}, err)
}

//export CPHCoreSync
func CPHCoreSync() *C.char {
	lock.Lock()
	defer lock.Unlock()
	if core == nil {
		return response(nil, fmt.Errorf("core not started"))
	}
	err := discover(coreDir)
	if err == nil {
		err = core.SyncPlatformPlugins(context.Background())
	}
	return response(map[string]bool{"ok": err == nil}, err)
}

//export CPHCoreRefresh
func CPHCoreRefresh(name *C.char) *C.char {
	lock.Lock()
	defer lock.Unlock()
	if core == nil {
		return response(map[string]bool{"ok": true}, nil)
	}
	err := discover(coreDir)
	if err == nil {
		err = core.RefreshPlatformPlugin(context.Background(), C.GoString(name))
	}
	return response(map[string]bool{"ok": err == nil}, err)
}

//export CPHCoreRuntime
func CPHCoreRuntime(raw *C.char) *C.char {
	lock.Lock()
	defer lock.Unlock()
	if core == nil {
		return response(nil, fmt.Errorf("core not started"))
	}
	var request app.NativeRuntimeRequest
	if err := json.Unmarshal([]byte(C.GoString(raw)), &request); err != nil {
		return response(nil, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	value, err := core.NativeRuntime(ctx, request)
	return response(value, err)
}

//export CPHCoreStop
func CPHCoreStop() *C.char {
	lock.Lock()
	defer lock.Unlock()
	if core == nil {
		return response(map[string]bool{"ok": true}, nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := core.Shutdown(ctx)
	if err == nil {
		core = nil
	}
	return response(map[string]bool{"ok": err == nil}, err)
}

//export CPHCoreDue
func CPHCoreDue(id C.longlong) *C.char {
	value, ok := jobs.Load(int64(id))
	if !ok {
		return response(nil, context.Canceled)
	}
	job := value.(systemJob)
	defer jobs.Delete(int64(id))
	defer job.cancel()
	lock.Lock()
	a := core
	lock.Unlock()
	if a == nil {
		return response(nil, fmt.Errorf("core unavailable"))
	}
	err := a.ExecuteDue(job.ctx)
	return response(map[string]bool{"ok": err == nil}, err)
}

//export CPHCoreCancel
func CPHCoreCancel(id C.longlong) {
	if value, ok := jobs.LoadAndDelete(int64(id)); ok {
		value.(systemJob).cancel()
	}
}
func main() {}
