package database

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extstore"
)

// ValidateBackup 只读完整性检查，并验证每个加密字段能用待恢复的密钥解开。
func ValidateBackup(ctx context.Context, path string, key []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	db, err := sql.Open("sqlite3", path+"?_query_only=1")
	if err != nil {
		return err
	}
	defer db.Close()
	var check string
	if err = db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&check); err != nil {
		return err
	}
	if check != "ok" {
		return fmt.Errorf("invalid SQLite backup: %s", check)
	}
	var count int
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('accounts','keys','users','settings')").Scan(&count); err != nil || count != 4 {
		return fmt.Errorf("backup is not a ClawProxyHub database")
	}
	var gcm cipher.AEAD
	if len(key) == 32 {
		block, _ := aes.NewCipher(key)
		gcm, _ = cipher.NewGCM(block)
	}
	for _, field := range [][2]string{{"accounts", "credential_blob"}, {"keys", "key_cipher"}, {"oauth_credentials", "token_blob"}, {"proxies", "password_cipher"}} {
		columns, err := db.QueryContext(ctx, "PRAGMA table_info("+field[0]+")")
		if err != nil {
			return err
		}
		exists := false
		for columns.Next() {
			var cid, notnull, pk int
			var name, typ string
			var def any
			if err = columns.Scan(&cid, &name, &typ, &notnull, &def, &pk); err != nil {
				columns.Close()
				return err
			}
			exists = exists || name == field[1]
		}
		err = columns.Err()
		columns.Close()
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		rows, err := db.QueryContext(ctx, "SELECT "+field[1]+" FROM "+field[0])
		if err != nil {
			return err
		}
		for rows.Next() {
			var data []byte
			if err = rows.Scan(&data); err != nil {
				rows.Close()
				return err
			}
			if len(data) == 0 || data[0] != 1 {
				continue
			}
			if gcm == nil || len(data) < 1+gcm.NonceSize()+gcm.Overhead() {
				rows.Close()
				return fmt.Errorf("backup requires a valid encryption key")
			}
			n := gcm.NonceSize()
			if _, err = gcm.Open(nil, data[1:1+n], data[1+n:], nil); err != nil {
				rows.Close()
				return fmt.Errorf("backup encryption key does not match %s", field[0])
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func effectiveRestoreKey(dataDir string) ([]byte, error) {
	if value := os.Getenv("CPH_SECRET_KEY"); value != "" {
		if b, e := hex.DecodeString(value); e == nil && len(b) == 32 {
			return b, nil
		}
		if len(value) == 32 {
			return []byte(value), nil
		}
		return nil, fmt.Errorf("invalid CPH_SECRET_KEY")
	}
	b, err := os.ReadFile(filepath.Join(dataDir, "secret.key"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// PublishRestore 先发布完整暂存集合，上传失败不污染上一次待恢复数据。
func PublishRestore(stage, dataDir string) error {
	target := filepath.Join(dataDir, "restore")
	old := filepath.Join(dataDir, ".restore-previous")
	if err := recoverPendingDirectory(dataDir); err != nil {
		return err
	}
	if err := os.RemoveAll(old); err != nil {
		return err
	}
	hadOld := fileExists(target)
	if hadOld {
		if err := os.Rename(target, old); err != nil {
			return err
		}
	}
	if err := os.Rename(stage, target); err != nil {
		if hadOld {
			if rb := os.Rename(old, target); rb != nil {
				return fmt.Errorf("%w; rollback: %v", err, rb)
			}
		}
		return err
	}
	return os.RemoveAll(old)
}

func recoverPendingDirectory(dataDir string) error {
	target := filepath.Join(dataDir, "restore")
	old := filepath.Join(dataDir, ".restore-previous")
	if !fileExists(target) && fileExists(old) {
		return os.Rename(old, target)
	}
	return nil
}

type restoreJournal struct {
	ID        string
	Originals map[string]bool
}

func finishRestore(dir string) error {
	completed := dir + ".applied-" + fmt.Sprint(time.Now().UnixNano())
	if err := os.Rename(dir, completed); err != nil {
		return err
	}
	return os.RemoveAll(completed)
}

func copyRestoreFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// ApplyPendingRestore 在启动前成组切换数据库和密钥；日志使中途崩溃在下次启动时回滚。
func ApplyPendingRestore(dbPath, dataDir string) error {
	if err := recoverPendingDirectory(dataDir); err != nil {
		return err
	}
	dir := filepath.Join(dataDir, "restore")
	pending := filepath.Join(dir, "cph.db")
	journalPath := filepath.Join(dir, "journal.json")
	targets := map[string]string{"db": dbPath, "wal": dbPath + "-wal", "shm": dbPath + "-shm", "key": filepath.Join(dataDir, "secret.key")}
	extensionPath := filepath.Join(dataDir, "cph.ext.db")
	targets["ext"], targets["extwal"], targets["extshm"] = extensionPath, extensionPath+"-wal", extensionPath+"-shm"
	rollback := func(j restoreJournal) error {
		if j.ID == "" || strings.ContainsAny(j.ID, "/\\.") {
			return fmt.Errorf("invalid restore journal")
		}
		for name, had := range j.Originals {
			target, ok := targets[name]
			if !ok {
				return fmt.Errorf("invalid restore target")
			}
			backup := target + ".bak-" + j.ID
			if fileExists(backup) {
				if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
					return err
				}
				if err := os.Rename(backup, target); err != nil {
					return err
				}
			} else if !had {
				if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
		}
		return os.Remove(journalPath)
	}
	if fileExists(filepath.Join(dir, "committed")) {
		return finishRestore(dir)
	}
	if data, err := os.ReadFile(journalPath); err == nil {
		var j restoreJournal
		if err = json.Unmarshal(data, &j); err != nil {
			return err
		}
		if err = rollback(j); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if !fileExists(pending) {
		return nil
	}
	key, err := effectiveRestoreKey(dataDir)
	if err != nil {
		return err
	}
	hasKey := fileExists(filepath.Join(dir, "secret.key"))
	if hasKey {
		incoming, e := os.ReadFile(filepath.Join(dir, "secret.key"))
		if e != nil {
			return e
		}
		if len(incoming) != 32 {
			return fmt.Errorf("invalid backup key")
		}
		if os.Getenv("CPH_SECRET_KEY") != "" && string(key) != string(incoming) {
			return fmt.Errorf("backup key differs from CPH_SECRET_KEY")
		}
		key = incoming
	}
	if err = ValidateBackup(context.Background(), pending, key); err != nil {
		return err
	}
	hasExtensions := fileExists(filepath.Join(dir, "cph.ext.db"))
	if hasExtensions {
		if err = extstore.ValidateBackup(context.Background(), filepath.Join(dir, "cph.ext.db")); err != nil {
			return err
		}
	}
	j := restoreJournal{ID: fmt.Sprint(time.Now().UnixNano()), Originals: make(map[string]bool)}
	for name, target := range targets {
		if strings.HasPrefix(name, "ext") && !hasExtensions {
			continue
		}
		if name == "key" && !hasKey {
			continue
		}
		j.Originals[name] = fileExists(target)
	}
	raw, _ := json.Marshal(j)
	f, err := os.OpenFile(journalPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	fail := func(cause error) error {
		if err := rollback(j); err != nil {
			return fmt.Errorf("restore failed: %w; rollback failed: %v", cause, err)
		}
		return cause
	}
	for name, had := range j.Originals {
		if had {
			target := targets[name]
			if err = os.Rename(target, target+".bak-"+j.ID); err != nil {
				return fail(err)
			}
		}
	}
	if err = copyRestoreFile(pending, dbPath); err != nil {
		return fail(err)
	}
	if hasExtensions {
		if err = copyRestoreFile(filepath.Join(dir, "cph.ext.db"), extensionPath); err != nil {
			return fail(err)
		}
	}
	if hasKey {
		if err = copyRestoreFile(filepath.Join(dir, "secret.key"), targets["key"]); err != nil {
			return fail(err)
		}
	}
	committed, err := os.OpenFile(filepath.Join(dir, "committed"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fail(err)
	}
	err = committed.Sync()
	closeErr = committed.Close()
	if err != nil {
		return fail(err)
	}
	if closeErr != nil {
		return fail(closeErr)
	}
	return finishRestore(dir)
}
