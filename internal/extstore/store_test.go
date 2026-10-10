package extstore

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

var allGrants = []string{"storage.read", "storage.write"}
var testHash = strings.Repeat("a", 64)

func testSchema(version int, names ...string) spec.StorageSchema {
	s := spec.StorageSchema{Version: version}
	for _, name := range names {
		s.Tables = append(s.Tables, spec.StorageTable{Name: name, Columns: []spec.StorageColumn{{Name: "id", Type: "string", PrimaryKey: true}, {Name: "content", Type: "string"}}, Indexes: []spec.StorageIndex{{Name: "by_content", Columns: []string{"content"}}}})
	}
	return s
}
func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "storage with spaces", "cph.ext.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func bindTest(t *testing.T, s *Store, id string, schema spec.StorageSchema) *Session {
	t.Helper()
	v, err := s.Bind(context.Background(), id, "publisher", testHash, schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	return v
}
func commitTest(t *testing.T, s *Store, id string, schema spec.StorageSchema) {
	t.Helper()
	if err := s.SyncInstalled(context.Background(), []Installed{{ID: id, Hash: testHash, Storage: &schema}}); err != nil {
		t.Fatal(err)
	}
}
func request(t *testing.T, v *Session, op, table string, values map[string]string) spec.StorageResult {
	t.Helper()
	r := spec.StorageRequest{Op: op, Table: table, Values: map[string]json.RawMessage{}}
	for name, value := range values {
		r.Values[name], _ = json.Marshal(value)
	}
	results, err := v.Execute(context.Background(), []spec.StorageRequest{r}, allGrants)
	if err != nil {
		t.Fatal(err)
	}
	return results[0]
}
func TestSharedDatabaseIsolationPersistenceAndAtomicBatch(t *testing.T) {
	s := openTest(t)
	schema := testSchema(1, "notes", "labels")
	a := bindTest(t, s, "alpha", schema)
	b := bindTest(t, s, "beta", schema)
	if err := s.SyncInstalled(context.Background(), []Installed{{ID: "alpha", Hash: testHash, Storage: &schema}, {ID: "beta", Hash: testHash, Storage: &schema}}); err != nil {
		t.Fatal(err)
	}
	payload := "'; DROP TABLE extension_storage_schemas; --"
	request(t, a, "insert", "notes", map[string]string{"id": "one", "content": payload})
	request(t, a, "insert", "labels", map[string]string{"content": "another table"})
	if result := request(t, b, "query", "notes", nil); len(result.Rows) != 0 {
		t.Fatal("cross-extension data leak")
	}
	if result := request(t, a, "query", "notes", nil); result.Rows[0]["content"] != payload {
		t.Fatal(result)
	}
	for _, r := range []spec.StorageRequest{{Op: "query", Table: "sqlite_master"}, {Op: "query", Table: a.tables["notes"]}, {Op: "query", Table: "notes", Columns: []string{"_cph_size"}}, {Op: "query", Table: "notes", Where: []spec.Predicate{{Column: "content", Op: "raw", Value: json.RawMessage(`"1=1"`)}}}} {
		if _, err := a.Execute(context.Background(), []spec.StorageRequest{r}, allGrants); err == nil {
			t.Fatal("accepted undeclared query", r)
		}
	}
	insert := spec.StorageRequest{Op: "insert", Table: "notes", Values: map[string]json.RawMessage{"id": json.RawMessage(`"duplicate"`), "content": json.RawMessage(`"x"`)}}
	if _, err := a.Execute(context.Background(), []spec.StorageRequest{insert, insert}, allGrants); err == nil {
		t.Fatal("duplicate batch should fail")
	}
	if result := request(t, a, "query", "notes", nil); len(result.Rows) != 1 {
		t.Fatal("partial batch committed")
	}
	if _, err := a.Execute(context.Background(), []spec.StorageRequest{insert}, []string{"storage.read"}); err == nil {
		t.Fatal("readonly write allowed")
	}
	a.Close()
	if _, err := a.Execute(context.Background(), []spec.StorageRequest{{Op: "query", Table: "notes"}}, allGrants); err == nil {
		t.Fatal("revoked session usable")
	}
	a = bindTest(t, s, "alpha", schema)
	if len(request(t, a, "query", "notes", nil).Rows) != 1 {
		t.Fatal("data did not survive session")
	}
	if err := s.Clear(context.Background(), "alpha"); err == nil {
		t.Fatal("cleared live session")
	}
	a.Close()
	if err := s.Clear(context.Background(), "alpha"); err != nil {
		t.Fatal(err)
	}
	request(t, b, "insert", "notes", map[string]string{"content": "still available"})
}

func TestObsoleteTablesOnlyChangeAtCommitAndRespectRollback(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	old := testSchema(1, "notes", "history")
	next := testSchema(2, "notes", "labels")
	v := bindTest(t, s, "alpha", old)
	commitTest(t, s, "alpha", old)
	request(t, v, "insert", "history", map[string]string{"content": "keep until explicit cleanup"})
	v.Close()
	candidate := bindTest(t, s, "alpha", next)
	if tables, err := s.Obsolete(ctx, "alpha", &old, false); err != nil || len(tables) != 0 {
		t.Fatal("candidate changed committed markers", tables, err)
	}
	candidate.Close()
	v = bindTest(t, s, "alpha", old)
	commitTest(t, s, "alpha", old)
	if tables, err := s.Obsolete(ctx, "alpha", &old, false); err != nil || len(tables) != 0 {
		t.Fatal("failed install left markers", tables, err)
	}
	v.Close()
	v = bindTest(t, s, "alpha", next)
	commitTest(t, s, "alpha", next)
	if tables, err := s.Obsolete(ctx, "alpha", &next, false); err != nil || !reflect.DeepEqual(tables, []string{"history"}) {
		t.Fatal(tables, err)
	}
	if _, err := v.Execute(ctx, []spec.StorageRequest{{Op: "query", Table: "history"}}, allGrants); err == nil {
		t.Fatal("new version accessed obsolete table")
	}
	v.Close()
	rollback := bindTest(t, s, "alpha", old)
	commitTest(t, s, "alpha", old)
	if tables, err := s.Obsolete(ctx, "alpha", &old, false); err != nil || !reflect.DeepEqual(tables, []string{"labels"}) {
		t.Fatal("rollback markers not recalculated", tables, err)
	}
	if len(request(t, rollback, "query", "history", nil).Rows) != 1 {
		t.Fatal("rollback lost retained data")
	}
	rollback.Close()
	v = bindTest(t, s, "alpha", next)
	commitTest(t, s, "alpha", next)
	if tables, err := s.Obsolete(ctx, "alpha", &next, true); err != nil || len(tables) != 1 {
		t.Fatal(tables, err)
	}
	request(t, v, "insert", "notes", map[string]string{"content": "active data"})
	v.Close()
	if _, err := s.Bind(ctx, "alpha", "publisher", testHash, old); err == nil {
		t.Fatal("rollback recreated cleaned table")
	}
	v = bindTest(t, s, "alpha", next)
	if len(request(t, v, "query", "notes", nil).Rows) != 1 {
		t.Fatal("obsolete cleanup touched active table")
	}
}

func TestMigrationValidationAndRetainedSigner(t *testing.T) {
	s := openTest(t)
	old := testSchema(1, "notes")
	v := bindTest(t, s, "alpha", old)
	commitTest(t, s, "alpha", old)
	request(t, v, "insert", "notes", map[string]string{"content": "saved"})
	v.Close()
	next := testSchema(2, "notes", "extra")
	next.Tables[0].Columns[1].Type = "integer"
	if _, err := s.Bind(context.Background(), "alpha", "publisher", testHash, next); err == nil {
		t.Fatal("type-changing migration accepted")
	}
	v = bindTest(t, s, "alpha", old)
	if len(request(t, v, "query", "notes", nil).Rows) != 1 {
		t.Fatal("failed migration changed rows")
	}
	v.Close()
	if _, err := s.Bind(context.Background(), "alpha", "other-signer", testHash, old); err == nil {
		t.Fatal("retained data transferred to another signer")
	}
	next = testSchema(2, "notes")
	next.Tables[0].Columns = append(next.Tables[0].Columns, spec.StorageColumn{Name: "flag", Type: "boolean", Default: json.RawMessage(`false`)})
	v = bindTest(t, s, "alpha", next)
	commitTest(t, s, "alpha", next)
	result := request(t, v, "query", "notes", nil)
	if result.Rows[0]["flag"] != false {
		t.Fatal("default not applied", result)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := v.Execute(cancelled, []spec.StorageRequest{{Op: "query", Table: "notes"}}, allGrants); err == nil {
		t.Fatal("cancelled call succeeded")
	}
}
