package database

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extstore"
)

func restoreFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
}

func restoreDatabase(t *testing.T, path, value string) {
	t.Helper()
	db := openSQLiteTest(t, context.Background(), path)
	if _, err := db.Exec("CREATE TABLE restore_test(value TEXT); INSERT INTO restore_test VALUES(?)", value); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreWithoutExtensionDatabasePreservesExtensionFiles(t *testing.T) {
	t.Setenv("CPH_SECRET_KEY", "")
	dir := t.TempDir()
	stage := filepath.Join(dir, "restore")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "cph.db")
	restoreDatabase(t, main, "old")
	restoreDatabase(t, filepath.Join(stage, "cph.db"), "restored")
	for _, name := range []string{"cph.ext.db", "cph.ext.db-wal", "cph.ext.db-shm"} {
		restoreFile(t, filepath.Join(dir, name), []byte("preserve "+name))
	}
	if err := ApplyPendingRestore(main, dir); err != nil {
		t.Fatal(err)
	}
	db := openSQLiteTest(t, context.Background(), main)
	var value string
	if err := db.QueryRow("SELECT value FROM restore_test").Scan(&value); err != nil || value != "restored" {
		t.Fatal("core database did not restore", value, err)
	}
	for _, name := range []string{"cph.ext.db", "cph.ext.db-wal", "cph.ext.db-shm"} {
		if content, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(content) != "preserve "+name {
			t.Fatal("legacy backup changed extension data", name, err)
		}
	}
}

func TestInvalidExtensionRestoreDoesNotChangeCoreOrKey(t *testing.T) {
	t.Setenv("CPH_SECRET_KEY", "")
	dir := t.TempDir()
	stage := filepath.Join(dir, "restore")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "cph.db")
	restoreDatabase(t, main, "old")
	restoreDatabase(t, filepath.Join(stage, "cph.db"), "candidate")
	store, err := extstore.Open(filepath.Join(dir, "cph.ext.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	restoreFile(t, filepath.Join(dir, "secret.key"), bytes.Repeat([]byte{1}, 32))
	restoreFile(t, filepath.Join(stage, "secret.key"), bytes.Repeat([]byte{2}, 32))
	restoreFile(t, filepath.Join(stage, "cph.ext.db"), []byte("not SQLite"))
	before := map[string][]byte{}
	for _, name := range []string{"cph.db", "cph.ext.db", "secret.key"} {
		before[name], err = os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = ApplyPendingRestore(main, dir); err == nil {
		t.Fatal("invalid extension backup was accepted")
	}
	for name, original := range before {
		if content, err := os.ReadFile(filepath.Join(dir, name)); err != nil || !bytes.Equal(content, original) {
			t.Fatal("failed restore changed original", name, err)
		}
	}
}

func TestInterruptedRestoreRollsBackExtensionDatabaseAndWAL(t *testing.T) {
	dir := t.TempDir()
	stage := filepath.Join(dir, "restore")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	targets := map[string]string{"db": "cph.db", "ext": "cph.ext.db", "extwal": "cph.ext.db-wal", "extshm": "cph.ext.db-shm", "key": "secret.key"}
	journal := restoreJournal{ID: "123", Originals: map[string]bool{}}
	for key, name := range targets {
		journal.Originals[key] = key != "extshm"
		if journal.Originals[key] {
			restoreFile(t, filepath.Join(dir, name+".bak-123"), []byte("original "+name))
		}
		restoreFile(t, filepath.Join(dir, name), []byte("partial replacement"))
	}
	raw, _ := json.Marshal(journal)
	restoreFile(t, filepath.Join(stage, "journal.json"), raw)
	if err := ApplyPendingRestore(filepath.Join(dir, "cph.db"), dir); err != nil {
		t.Fatal(err)
	}
	for key, name := range targets {
		content, err := os.ReadFile(filepath.Join(dir, name))
		if key == "extshm" {
			if !os.IsNotExist(err) {
				t.Fatal("rollback retained a new shared-memory file", err)
			}
		} else if err != nil || string(content) != "original "+name {
			t.Fatal("incomplete rollback", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(stage, "journal.json")); !os.IsNotExist(err) {
		t.Fatal("rollback journal remains", err)
	}
}
