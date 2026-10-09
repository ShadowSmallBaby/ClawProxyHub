// Package extstore 由宿主独占 cph.ext.db，扩展只持有可撤销的逻辑表会话。
package extstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/*.sql
var migrations embed.FS

var ErrUnavailable = errors.New("extension storage session unavailable")
var ErrBusy = errors.New("extension storage queue is full")

type Store struct {
	db       *sql.DB
	slots    chan struct{}
	serial   chan struct{}
	mu       sync.Mutex
	sessions map[string]*Session
	closed   bool
}
type Session struct {
	store  *Store
	id     string
	schema spec.StorageSchema
	tables map[string]string
	ctx    context.Context
	cancel context.CancelFunc
}
type record struct {
	id, signer, namespace, hash, applied string
	schema                               spec.StorageSchema
	tables                               map[string]string
	history                              map[string]revision
	definitions                          map[string]spec.StorageTable
	committed                            map[string]bool
	active                               *spec.StorageSchema
	obsolete                             []string
	used                                 int64
}
type revision struct {
	Hash      string   `json:"hash"`
	Tables    []string `json:"tables"`
	Committed bool     `json:"committed,omitempty"`
}
type Installed struct {
	ID, Hash string
	Storage  *spec.StorageSchema
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	uriPath := filepath.ToSlash(absolute)
	if filepath.VolumeName(absolute) != "" {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	db, err := sql.Open("sqlite3", u.String()+"?_journal_mode=WAL&_busy_timeout=2000&_foreign_keys=on&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	fail := func(err error) (*Store, error) { db.Close(); return nil, err }
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return fail(err)
	}
	source, err := iofs.New(migrations, "migrations")
	if err != nil {
		return fail(err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "sqlite3", driver)
	if err != nil {
		return fail(err)
	}
	if err = m.Up(); err != nil && err != migrate.ErrNoChange {
		return fail(err)
	}
	// 迁移对象复用本库连接，容量按实际页大小限制为 1 GiB。
	var pageSize int64
	if err = db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil || pageSize < 512 {
		return fail(fmt.Errorf("invalid extension database page size"))
	}
	for _, statement := range []string{fmt.Sprintf("PRAGMA max_page_count=%d", (1<<30)/pageSize), "PRAGMA wal_autocheckpoint=1000", "PRAGMA journal_size_limit=8388608"} {
		if _, err = db.Exec(statement); err != nil {
			return fail(err)
		}
	}
	if err = os.Chmod(path, 0600); err != nil {
		return fail(err)
	}
	return &Store{db: db, slots: make(chan struct{}, 32), serial: make(chan struct{}, 1), sessions: map[string]*Session{}}, nil
}

func (s *Store) acquire(ctx context.Context) (func(), error) {
	select {
	case s.slots <- struct{}{}:
	default:
		return nil, ErrBusy
	}
	select {
	case s.serial <- struct{}{}:
		return func() { <-s.serial; <-s.slots }, nil
	case <-ctx.Done():
		<-s.slots
		return nil, ctx.Err()
	}
}
func (s *Store) transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	release, err := s.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
func get(ctx context.Context, tx *sql.Tx, id string) (*record, error) {
	r := &record{id: id}
	var schema, tables, history, definitions, committed, active, obsolete string
	err := tx.QueryRowContext(ctx, `SELECT signer,namespace,schema_hash,schema_json,table_map,history_json,used_bytes,applied_package,tables_json,committed_json,active_schema,obsolete_json FROM extension_storage_schemas WHERE extension_id=?`, id).Scan(&r.signer, &r.namespace, &r.hash, &schema, &tables, &history, &r.used, &r.applied, &definitions, &committed, &active, &obsolete)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = spec.DecodeStrict([]byte(schema), &r.schema); err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(tables), &r.tables); err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(history), &r.history); err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(definitions), &r.definitions); err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(committed), &r.committed); err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(active), &r.active); err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(obsolete), &r.obsolete); err != nil {
		return nil, err
	}
	_, hash, err := r.schema.Canonical()
	if err != nil || hash != r.hash || len(r.namespace) != 32 {
		return nil, fmt.Errorf("invalid stored extension schema")
	}
	if _, err := hex.DecodeString(r.namespace); err != nil {
		return nil, err
	}
	for name, t := range r.definitions {
		if name != t.Name || (spec.StorageSchema{Version: 1, Tables: []spec.StorageTable{t}}).Validate() != nil || r.tables[t.Name] != physical(r.namespace, t.Name) {
			return nil, fmt.Errorf("invalid stored table mapping")
		}
	}
	if len(r.tables) != len(r.definitions) || len(r.tables) > 64 {
		return nil, fmt.Errorf("invalid stored table count")
	}
	return r, nil
}
func physical(namespace, name string) string { return "e_" + namespace + "_" + name }
func quote(name string) string               { return `"` + name + `"` }
func sqlType(c spec.StorageColumn) string {
	switch c.Type {
	case "integer", "boolean":
		return "INTEGER"
	case "number":
		return "REAL"
	case "bytes":
		return "BLOB"
	default:
		return "TEXT"
	}
}
func literal(v any) string {
	switch value := v.(type) {
	case nil:
		return "NULL"
	case string:
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	case []byte:
		return "X'" + hex.EncodeToString(value) + "'"
	case int64:
		return strconv.FormatInt(value, 10)
	case float64:
		return strconv.FormatFloat(value, 'g', -1, 64)
	default:
		panic("validated storage value required")
	}
}
func columnSQL(c spec.StorageColumn) string {
	text := quote(c.Name) + " " + sqlType(c)
	if c.PrimaryKey {
		text += " PRIMARY KEY"
	}
	if !c.Nullable {
		text += " NOT NULL"
	}
	if len(c.Default) > 0 {
		v, _ := c.Value(c.Default)
		text += " DEFAULT " + literal(v)
	}
	return text
}
func createTable(t spec.StorageTable, name string) []string {
	columns := []string{}
	for _, c := range t.Columns {
		columns = append(columns, columnSQL(c))
	}
	columns = append(columns, `"_cph_size" INTEGER NOT NULL DEFAULT 0`)
	statements := []string{"CREATE TABLE " + quote(name) + " (" + strings.Join(columns, ",") + ")"}
	for _, index := range t.Indexes {
		statements = append(statements, createIndex(index, name))
	}
	return statements
}
func createIndex(index spec.StorageIndex, table string) string {
	cols := []string{}
	for _, c := range index.Columns {
		cols = append(cols, quote(c))
	}
	unique := ""
	if index.Unique {
		unique = "UNIQUE "
	}
	return "CREATE " + unique + "INDEX " + quote(table+"_i_"+index.Name) + " ON " + quote(table) + " (" + strings.Join(cols, ",") + ")"
}
func equal(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// changes 只生成兼容的新增结构，旧版后端可继续使用自己的声明快照。
func changes(r *record, next spec.StorageSchema) ([]string, error) {
	statements := []string{}
	oldTables := r.definitions
	count := len(oldTables)
	for _, t := range next.Tables {
		name := physical(r.namespace, t.Name)
		old, exists := oldTables[t.Name]
		if !exists {
			count++
			if count > 64 {
				return nil, fmt.Errorf("retained table limit reached; clean obsolete tables first")
			}
			statements = append(statements, createTable(t, name)...)
			continue
		}
		columns := map[string]spec.StorageColumn{}
		for _, c := range t.Columns {
			columns[c.Name] = c
		}
		for _, c := range old.Columns {
			if !equal(columns[c.Name], c) {
				return nil, fmt.Errorf("column removal or modification requires a supported migration: %s.%s", t.Name, c.Name)
			}
			delete(columns, c.Name)
		}
		for _, c := range t.Columns {
			if _, add := columns[c.Name]; add {
				if c.PrimaryKey || (!c.Nullable && len(c.Default) == 0) {
					return nil, fmt.Errorf("new columns require nullable or literal default")
				}
				statements = append(statements, "ALTER TABLE "+quote(name)+" ADD COLUMN "+columnSQL(c))
			}
		}
		indexes := map[string]spec.StorageIndex{}
		for _, index := range t.Indexes {
			indexes[index.Name] = index
		}
		for _, index := range old.Indexes {
			if !equal(indexes[index.Name], index) {
				return nil, fmt.Errorf("index removal or modification is not supported")
			}
			delete(indexes, index.Name)
		}
		for _, index := range t.Indexes {
			if _, add := indexes[index.Name]; add {
				if index.Unique {
					return nil, fmt.Errorf("adding a unique constraint is not supported")
				}
				statements = append(statements, createIndex(index, name))
			}
		}
	}
	return statements, nil
}

func prepare(r *record, signer string, next spec.StorageSchema, hash string) ([]string, error) {
	if r.signer != signer {
		return nil, fmt.Errorf("retained storage belongs to another signing identity; explicit data cleanup is required")
	}
	if next.Version <= r.schema.Version {
		if r.history[strconv.Itoa(next.Version)].Hash != hash {
			return nil, fmt.Errorf("schema version already has different content")
		}
		if !r.history[strconv.Itoa(next.Version)].Committed && next.Version == r.schema.Version {
			return changes(r, next)
		}
		for _, table := range next.Tables {
			actual, ok := r.definitions[table.Name]
			if !ok {
				return nil, fmt.Errorf("schema requires a table that has been cleaned")
			}
			columns := columnsOf(actual)
			for _, column := range table.Columns {
				if !equal(columns[column.Name], column) {
					return nil, fmt.Errorf("stored columns are incompatible with this schema")
				}
			}
		}
		return nil, nil
	}
	if len(r.history) >= 128 {
		return nil, fmt.Errorf("storage schema history limit reached")
	}
	return changes(r, next)
}

func (s *Store) Check(ctx context.Context, id, signer string, schema *spec.StorageSchema) error {
	if schema == nil {
		return nil
	}
	raw, hash, err := schema.Canonical()
	if err != nil {
		return err
	}
	var next spec.StorageSchema
	_ = json.Unmarshal(raw, &next)
	return s.transaction(ctx, func(tx *sql.Tx) error {
		r, err := get(ctx, tx, id)
		if err != nil || r == nil {
			return err
		}
		_, err = prepare(r, signer, next, hash)
		return err
	})
}

func (s *Store) Bind(ctx context.Context, id, signer, packageHash string, schema spec.StorageSchema) (*Session, error) {
	if !spec.ValidID(id) || signer == "" || !spec.ValidHash(packageHash) {
		return nil, fmt.Errorf("invalid storage owner")
	}
	raw, hash, err := schema.Canonical()
	if err != nil {
		return nil, err
	}
	var next spec.StorageSchema
	_ = json.Unmarshal(raw, &next)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.sessions[id] != nil {
		return nil, ErrUnavailable
	}
	var tables map[string]string
	err = s.transaction(ctx, func(tx *sql.Tx) error {
		r, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		var statements []string
		if r == nil {
			random := make([]byte, 16)
			if _, err = rand.Read(random); err != nil {
				return err
			}
			r = &record{id: id, signer: signer, namespace: hex.EncodeToString(random), history: map[string]revision{}, tables: map[string]string{}, definitions: map[string]spec.StorageTable{}}
			for _, t := range next.Tables {
				statements = append(statements, createTable(t, physical(r.namespace, t.Name))...)
			}
		} else {
			statements, err = prepare(r, signer, next, hash)
			if err != nil {
				return err
			}
		}
		for _, statement := range statements {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply extension schema: %w", err)
			}
		}
		if next.Version > r.schema.Version || (next.Version == r.schema.Version && !r.history[strconv.Itoa(next.Version)].Committed) {
			r.schema = next
			r.hash = hash
			r.applied = packageHash
			rev := revision{Hash: hash}
			for _, t := range next.Tables {
				r.tables[t.Name] = physical(r.namespace, t.Name)
				r.definitions[t.Name] = t
				rev.Tables = append(rev.Tables, t.Name)
			}
			r.history[strconv.Itoa(next.Version)] = rev
			used, err := recount(ctx, tx, r)
			if err != nil {
				return err
			}
			r.used = used
			if r.used > spec.MaxStorageBytes {
				return fmt.Errorf("extension storage quota exceeded by migration")
			}
			mapping, _ := json.Marshal(r.tables)
			history, _ := json.Marshal(r.history)
			definitions, _ := json.Marshal(r.definitions)
			_, err = tx.ExecContext(ctx, `INSERT INTO extension_storage_schemas(extension_id,signer,namespace,schema_version,schema_hash,schema_json,table_map,history_json,used_bytes,applied_package,phase,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(extension_id) DO UPDATE SET schema_version=excluded.schema_version,schema_hash=excluded.schema_hash,schema_json=excluded.schema_json,table_map=excluded.table_map,history_json=excluded.history_json,used_bytes=excluded.used_bytes,applied_package=excluded.applied_package,phase=excluded.phase,updated_at=excluded.updated_at`, id, signer, r.namespace, next.Version, hash, string(raw), string(mapping), string(history), r.used, packageHash, "applied", time.Now().UTC().Format(time.RFC3339Nano))
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE extension_storage_schemas SET tables_json=? WHERE extension_id=?`, string(definitions), id); err != nil {
				return err
			}
		}
		tables = r.tables
		return nil
	})
	if err != nil {
		return nil, err
	}
	sessionCtx, cancel := context.WithCancel(context.Background())
	session := &Session{store: s, id: id, schema: next, tables: tables, ctx: sessionCtx, cancel: cancel}
	s.sessions[id] = session
	return session, nil
}

func (v *Session) Close() {
	v.cancel()
	s := v.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[v.id] == v {
		delete(s.sessions, v.id)
	}
}

func (s *Store) Clear(ctx context.Context, id string) error {
	if !spec.ValidID(id) {
		return fmt.Errorf("invalid extension ID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[id] != nil {
		return ErrUnavailable
	}
	return s.transaction(ctx, func(tx *sql.Tx) error {
		r, err := get(ctx, tx, id)
		if err != nil || r == nil {
			return err
		}
		for _, name := range r.tables {
			if _, err = tx.ExecContext(ctx, "DROP TABLE "+quote(name)); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM extension_storage_schemas WHERE extension_id=?`, id)
		return err
	})
}
func (s *Store) Snapshot(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	release, err := s.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	_, err = s.db.ExecContext(ctx, "VACUUM INTO ?", path)
	return err
}
func (s *Store) Close() error {
	s.mu.Lock()
	s.closed = true
	for _, session := range s.sessions {
		session.cancel()
	}
	s.sessions = map[string]*Session{}
	s.mu.Unlock()
	return s.db.Close()
}
