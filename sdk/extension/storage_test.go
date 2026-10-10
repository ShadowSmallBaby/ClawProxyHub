package extension

import (
	"encoding/json"
	"testing"
)

func TestStorageValuesPreserveTypesAndChronologicalOrder(t *testing.T) {
	timestamp := StorageColumn{Name: "created_at", Type: "datetime"}
	first, err := timestamp.Value(json.RawMessage(`"2026-10-09T08:00:00+08:00"`))
	if err != nil {
		t.Fatal(err)
	}
	next, err := timestamp.Value(json.RawMessage(`"2026-10-09T00:00:00.001Z"`))
	if err != nil || first.(string) >= next.(string) {
		t.Fatal("timestamps will not sort chronologically", first, next, err)
	}
	for _, test := range []struct{ kind, raw string }{
		{"integer", `9007199254740992`}, {"integer", `1.5`}, {"boolean", `1`},
		{"number", `1e999`}, {"datetime", `"not a date"`}, {"bytes", `"not-base64"`}, {"string", `null`},
	} {
		if _, err := (StorageColumn{Name: "value", Type: test.kind}).Value(json.RawMessage(test.raw)); err == nil {
			t.Fatalf("invalid %s value accepted: %s", test.kind, test.raw)
		}
	}
}

func TestStorageSchemaCanonicalizesOrderAndLiteralDefaults(t *testing.T) {
	first := StorageSchema{Version: 1, Tables: []StorageTable{{Name: "notes", Columns: []StorageColumn{{Name: "id", Type: "string", PrimaryKey: true}, {Name: "created_at", Type: "datetime", Default: json.RawMessage(`"2026-10-09T08:00:00+08:00"`)}}}}}
	_, hash, err := first.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	first.Tables[0].Columns[1].Default = json.RawMessage(`"2026-10-09T00:00:00.000000000Z"`)
	columns := first.Tables[0].Columns
	columns[0], columns[1] = columns[1], columns[0]
	_, reordered, err := first.Canonical()
	if err != nil || hash != reordered {
		t.Fatal("equivalent declarations have different identities", err)
	}
	for _, name := range []string{"../notes", `notes";DROP TABLE notes;--`, "sqlite_master", "_cph_size", "extension_storage_schemas"} {
		first.Tables[0].Name = name
		if err := first.Validate(); err == nil {
			t.Fatalf("unsafe table name accepted: %s", name)
		}
	}
}
