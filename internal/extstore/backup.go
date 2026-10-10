package extstore

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// ValidateBackup 只读检查扩展库、结构登记和表归属，不运行备份中的迁移或扩展代码。
func ValidateBackup(ctx context.Context, path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	uriPath := filepath.ToSlash(absolute)
	if filepath.VolumeName(absolute) != "" {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	db, err := sql.Open("sqlite3", u.String()+"?mode=ro&immutable=1")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var integrity string
	if err = db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		return fmt.Errorf("invalid extension database backup")
	}
	var version int
	var dirty bool
	if err = db.QueryRowContext(ctx, "SELECT version,dirty FROM schema_migrations").Scan(&version, &dirty); err != nil || dirty || version != 3 {
		return fmt.Errorf("unsupported extension database schema")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT extension_id FROM extension_storage_schemas")
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	allowed := map[string]bool{"schema_migrations": true, "extension_storage_schemas": true}
	for _, id := range ids {
		r, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		for _, table := range r.tables {
			if allowed[table] {
				return fmt.Errorf("duplicate storage owner")
			}
			allowed[table] = true
		}
	}
	rows, err = tx.QueryContext(ctx, `SELECT type,name,tbl_name FROM sqlite_master WHERE type IN ('table','view','trigger')`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, name, table string
		if err = rows.Scan(&kind, &name, &table); err != nil {
			return err
		}
		if kind != "table" || (!allowed[name] && !strings.HasPrefix(name, "sqlite_")) {
			return fmt.Errorf("unregistered extension database object")
		}
		delete(allowed, name)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(allowed) != 0 {
		return fmt.Errorf("missing registered extension table")
	}
	return nil
}
