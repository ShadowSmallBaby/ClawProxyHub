package plugin

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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
