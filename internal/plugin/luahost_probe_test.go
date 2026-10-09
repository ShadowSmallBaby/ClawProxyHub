//go:build !android

package plugin

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestLuaHostProbeStartsRealRuntimeAndRejectsInvalidExecutable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	file := filepath.Join(t.TempDir(), "luahost")
	if runtime.GOOS == "windows" {
		file += ".exe"
	}
	invalid := file + ".invalid"
	if err := os.WriteFile(invalid, []byte("invalid runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := ProbeLuaExecutable(ctx, invalid); err == nil {
		t.Fatal("non-executable runtime accepted")
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", file, "./cmd")
	cmd.Dir = filepath.Join("..", "..", "hosts", "luahost")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build Lua Host: %v\n%s", err, output)
	}
	if err := ProbeLuaExecutable(ctx, file); err != nil {
		t.Fatal(err)
	}
}
