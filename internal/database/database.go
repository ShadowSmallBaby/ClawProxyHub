// Package database — 连接管理与启动时自动迁移（migrations/*.sql 严格 up/down 配对）。
package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

//go:embed migrations/*.sql
var sqliteMigrations embed.FS

var (
	migrationStateMu        sync.RWMutex
	currentMigrationVersion uint
	currentMigrationDirty   bool
	migrationVersionSet     bool
	currentMigrationError   string
)

// CachedMigrationVersion 返回启动时捕获的迁移状态（版本, dirty, 是否已知）。
func CachedMigrationVersion() (uint, bool, bool) {
	migrationStateMu.RLock()
	defer migrationStateMu.RUnlock()
	return currentMigrationVersion, currentMigrationDirty, migrationVersionSet
}

// CachedMigrationError 返回最近一次迁移失败的错误信息，空串表示成功或未运行。
func CachedMigrationError() string {
	migrationStateMu.RLock()
	defer migrationStateMu.RUnlock()
	return currentMigrationError
}

func setMigrationState(version uint, dirty bool, errMsg string, known bool) {
	migrationStateMu.Lock()
	defer migrationStateMu.Unlock()
	if known {
		currentMigrationVersion = version
		currentMigrationDirty = dirty
		migrationVersionSet = true
	}
	currentMigrationError = errMsg
}

func captureMigrationFailure(m *migrate.Migrate, err error) error {
	known := false
	var ver uint
	var dirty bool
	if m != nil {
		if v, d, vErr := m.Version(); vErr == nil {
			known, ver, dirty = true, v, d
		}
	}
	setMigrationState(ver, dirty, err.Error(), known)
	return err
}

// Open 打开 SQLite 并执行启动迁移，返回 GORM 句柄。
// dsn 为 SQLite 文件路径。迁移失败返回错误，调用方应拒绝启动服务。
func Open(ctx context.Context, dsn string) (*gorm.DB, error) {
	// 迁移走独立连接：migrate.Migrate.Close 会关掉持有的 *sql.DB
	if err := runMigrations(dsn); err != nil {
		return nil, err
	}

	// WAL + busy_timeout：避免并发读写锁冲突
	sqlDB, err := sql.Open("sqlite3", dsn+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	gdb, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{
		// 记录不存在属正常业务路径（首次读设置等），不打 error 日志
		Logger: gormlogger.New(log.New(os.Stdout, "\r\n", log.LstdFlags),
			gormlogger.Config{
				SlowThreshold:             200 * time.Millisecond,
				LogLevel:                  gormlogger.Warn,
				IgnoreRecordNotFoundError: true,
				Colorful:                  true,
			}),
	})
	if err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("init gorm: %w", err)
	}
	return gdb, nil
}

// runMigrations 执行嵌入的 SQL 迁移；dirty 时报错并给 force 指引，不自动恢复。
func runMigrations(dsn string) error {
	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("open sqlite for migration: %w", err)
	}
	defer sqlDB.Close()

	driver, err := sqlite3.WithInstance(sqlDB, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("create migrate driver: %w", err)
	}
	src, err := iofs.New(sqliteMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("load embedded migrations: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite3", driver)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	defer m.Close()

	oldVersion, oldDirty, versionErr := m.Version()
	if versionErr != nil && versionErr != migrate.ErrNilVersion {
		return captureMigrationFailure(m, fmt.Errorf("get migration version: %w", versionErr))
	}
	if oldDirty {
		forceVersion := int(oldVersion) - 1
		if forceVersion < 0 {
			forceVersion = 0
		}
		return captureMigrationFailure(m, fmt.Errorf(
			"database is dirty at version %d (migration failed partway). "+
				"Fix manually then run: cph migrate force %d", oldVersion, forceVersion))
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return captureMigrationFailure(m, fmt.Errorf("run migrations: %w", err))
	}

	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return captureMigrationFailure(m, fmt.Errorf("get migration version: %w", err))
	}
	setMigrationState(version, dirty, "", true)

	if oldVersion != version {
		fmt.Printf("[database] migrated %d -> %d\n", oldVersion, version)
	} else {
		fmt.Printf("[database] up to date (version %d)\n", version)
	}
	return nil
}

// DSNToFilepath 从 DSN 提取 SQLite 文件路径（兼容带参数形式）。
func DSNToFilepath(dsn string) string {
	// 形如 /path/to/cph.db?_journal_mode=WAL
	if i := strings.IndexByte(dsn, '?'); i >= 0 {
		dsn = dsn[:i]
	}
	// Windows 下 file: URI
	if strings.HasPrefix(dsn, "file:") {
		if u, err := url.Parse(dsn); err == nil {
			return u.Path
		}
	}
	return dsn
}

// ApplyPendingRestore 启动时（打开库之前）换入管理界面上传的备份：
// <dataDir>/restore/cph.db 存在 → 当前库另存 cph.db.bak-<时间>，暂存文件移入；secret.key 同理。
// 换入后清理 -wal/-shm 残留，避免旧日志混入新库。
func ApplyPendingRestore(dbPath, dataDir string) error {
	dir := filepath.Join(dataDir, "restore")
	pending := filepath.Join(dir, "cph.db")
	if _, err := os.Stat(pending); err != nil {
		return nil
	}
	stamp := time.Now().Format("20060102-150405")
	if _, err := os.Stat(dbPath); err == nil {
		if err := os.Rename(dbPath, dbPath+".bak-"+stamp); err != nil {
			return fmt.Errorf("backup current db: %w", err)
		}
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		os.Remove(dbPath + suffix)
	}
	if err := os.Rename(pending, dbPath); err != nil {
		return fmt.Errorf("move restored db: %w", err)
	}
	if key := filepath.Join(dir, "secret.key"); fileExists(key) {
		dst := filepath.Join(dataDir, "secret.key")
		if fileExists(dst) {
			os.Rename(dst, dst+".bak-"+stamp)
		}
		os.Rename(key, dst)
	}
	os.RemoveAll(dir)
	fmt.Printf("[database] restored backup into %s (previous saved as .bak-%s)\n", dbPath, stamp)
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
