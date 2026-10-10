package plugin

import (
	"archive/zip"
	"context"
	"errors"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Lua 包经平台服务安装并握手，无需桌面 luahost 文件。
func TestInstallLuaWithServiceRuntime(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "lua.cphplugin")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	for name, content := range map[string]string{"manifest.json": `{"name":"lua-install","version":"1.0.0","runtime":"lua","protocol_version":2}`, "main.lua": "return {}", "lib/helpers.lua": "return {}"} {
		writer, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}
	file.Close()
	root := t.TempDir()
	connector := &pipeConnector{name: "lua-install", protocol: sdk.ProtocolVersion}
	manager := NewManager(root, nil, WithRuntime(NewServiceRuntime(connector)))
	if err := manager.SetLuaRuntime(func() (string, error) { return "verified-service", nil }); err != nil {
		t.Fatal(err)
	}
	defer manager.StopAll()
	if _, err = manager.InstallZip(context.Background(), archive, "", nil); err != nil {
		t.Fatal(err)
	}
	if connector.started.Load() != 1 {
		t.Fatal("platform runtime not used")
	}
	if _, err = os.Stat(filepath.Join(root, "lua-install", "lib", "helpers.lua")); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "hosts")); !os.IsNotExist(err) {
		t.Fatal("desktop host unexpectedly required")
	}
}

// 普通 ZIP 目录条目可通过校验，目录穿越仍拒绝；取消安装不触碰目标目录。
func TestInstallZipDirectoryEntries(t *testing.T) {
	for _, entry := range []string{"lib/", "../"} {
		t.Run(entry, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "test.cphplugin")
			file, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			zw := zip.NewWriter(file)
			binary := "plugin-" + runtime.GOOS + "-" + runtime.GOARCH
			if runtime.GOOS == "windows" {
				binary += ".exe"
			}
			for _, item := range [][2]string{{entry, ""}, {"manifest.json", `{"name":"directory-test","version":"1.0.0"}`}, {binary, "not launched"}} {
				writer, err := zw.Create(item[0])
				if err != nil {
					t.Fatal(err)
				}
				if _, err := writer.Write([]byte(item[1])); err != nil {
					t.Fatal(err)
				}
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			manager := NewManager(root, nil)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err = manager.InstallZip(ctx, archive, "", nil)
			if entry == "lib/" && !errors.Is(err, context.Canceled) {
				t.Fatalf("normal directory rejected before cancellation: %v", err)
			}
			if entry == "../" && (err == nil || !strings.Contains(err.Error(), "unsafe archive path")) {
				t.Fatalf("unsafe directory accepted: %v", err)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatalf("failed install left files: %v, %v", entries, err)
			}
		})
	}
}
