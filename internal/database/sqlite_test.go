package database

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// 用实际文件库验证迁移、WAL 和事务持久化，临时文件由测试框架清理。
func TestSQLitePersistence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "persistence.db")
	db := openSQLiteTest(t, ctx, path)
	var journal string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil || journal != "wal" {
		t.Fatalf("journal=%q, err=%v", journal, err)
	}
	var version uint
	var dirty bool
	if err := db.QueryRowContext(ctx, "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil || version == 0 || dirty {
		t.Fatalf("migration=%d, dirty=%v, err=%v", version, dirty, err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE persistence_test (value TEXT NOT NULL); INSERT INTO persistence_test VALUES ('committed')"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT INTO persistence_test VALUES ('rolled-back')"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openSQLiteTest(t, ctx, path)
	var count int
	var value string
	if err := reopened.QueryRowContext(ctx, "SELECT count(*), min(value) FROM persistence_test").Scan(&count, &value); err != nil || count != 1 || value != "committed" {
		t.Fatalf("persisted count=%d, value=%q, err=%v", count, value, err)
	}
	checkSQLiteIntegrity(t, ctx, reopened)
}

// 子进程不关闭连接和事务就退出，父进程验证已提交写入恢复且未提交写入丢弃。
func TestSQLiteRecoveryAfterProcessExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const childDirectory = "CPH_SQLITE_RECOVERY_CHILD"
	if dir := os.Getenv(childDirectory); dir != "" {
		db := openSQLiteTest(t, ctx, filepath.Join(dir, "recovery.db"))
		if _, err := db.ExecContext(ctx, "CREATE TABLE recovery_test (value TEXT NOT NULL); INSERT INTO recovery_test VALUES ('committed')"); err != nil {
			t.Fatal(err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE recovery_test SET value='uncommitted'"); err != nil {
			t.Fatal(err)
		}
		os.Exit(0)
	}
	dir := t.TempDir()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSQLiteRecoveryAfterProcessExit$")
	child.Env = append(os.Environ(), childDirectory+"="+dir)
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("child process: %s, %v", output, err)
	}
	db := openSQLiteTest(t, ctx, filepath.Join(dir, "recovery.db"))
	var count int
	var value string
	if err := db.QueryRowContext(ctx, "SELECT count(*), min(value) FROM recovery_test").Scan(&count, &value); err != nil || count != 1 || value != "committed" {
		t.Fatalf("recovered count=%d, value=%q, err=%v", count, value, err)
	}
	checkSQLiteIntegrity(t, ctx, db)
}

func openSQLiteTest(t *testing.T, ctx context.Context, path string) *sql.DB {
	t.Helper()
	gdb, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	db, err := gdb.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func checkSQLiteIntegrity(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity=%q, err=%v", integrity, err)
	}
}
