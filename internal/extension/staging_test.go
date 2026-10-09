package extension

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadTicketBindsVerifiedBytesAndIsConsumed(t *testing.T) {
	f := newFixture(t)
	m := New(t.TempDir(), "1.5.2", f.trust)
	ctx := context.Background()
	if err := m.Load(ctx); err != nil {
		t.Fatal(err)
	}
	source := f.pack("editor", "1.0.0", nil, nil)
	ticket, err := m.StageUpload(source)
	if err != nil {
		t.Fatal(err)
	}
	// 源文件随后改变不能替换已经核验的暂存内容。
	if err := os.WriteFile(source, []byte("changed after inspection"), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := m.InstallUpload(ctx, ticket, []string{"workspace.read"})
	if err != nil || state.Manifest.ID != "editor" {
		t.Fatalf("install: %v, %v", state, err)
	}
	if _, err = m.InstallUpload(ctx, ticket, []string{"workspace.read"}); err == nil {
		t.Fatal("consumed ticket accepted")
	}
	for _, invalid := range []string{"../editor", source, strings.Repeat("a", 63)} {
		if _, err = m.InstallUpload(ctx, invalid, nil); err == nil {
			t.Fatal("invalid ticket accepted")
		}
	}
}

func TestUploadTamperingAndLockedHashRejectBeforeActivation(t *testing.T) {
	f := newFixture(t)
	m := New(t.TempDir(), "1.5.2", f.trust)
	ctx := context.Background()
	activated := false
	m.SetLifecycle(func(context.Context, State) error { activated = true; return nil }, nil)
	source := f.pack("editor", "1.0.0", nil, nil)
	if _, err := m.InstallExpected(ctx, source, []string{"workspace.read"}, strings.Repeat("0", 64)); err == nil {
		t.Fatal("locked hash mismatch accepted")
	}
	ticket, err := m.StageUpload(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.dir, "uploads", ticket+".cphext")
	if err = os.WriteFile(path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = m.InstallUpload(ctx, ticket, []string{"workspace.read"}); err == nil {
		t.Fatal("tampered upload accepted")
	}
	if activated {
		t.Fatal("rejected upload activated code")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("failed upload was not discarded")
	}
}
