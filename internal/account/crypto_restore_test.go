package account

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRestartReloadsRestoredMasterKey(t *testing.T) {
	t.Setenv("CPH_SECRET_KEY", "")
	dir := t.TempDir()
	first, err := EncryptCredential(dir, []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{0x57}, 32)
	if err = os.WriteFile(filepath.Join(dir, "secret.key"), key, 0600); err != nil {
		t.Fatal(err)
	}
	if err = InitCrypto(dir); err != nil {
		t.Fatal(err)
	}
	if _, err = DecryptCredential(dir, first); err == nil {
		t.Fatal("retained stale key after restart")
	}
	encrypted, err := EncryptCredential(dir, []byte("restored"))
	if err != nil {
		t.Fatal(err)
	}
	if err = InitCrypto(dir); err != nil {
		t.Fatal(err)
	}
	plain, err := DecryptCredential(dir, encrypted)
	if err != nil || string(plain) != "restored" {
		t.Fatalf("%s %v", plain, err)
	}
}
