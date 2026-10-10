// storage.go — 扩展只声明逻辑结构和有界数据请求，连接及 SQL 由宿主管理。
package extension

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	MaxTables       = 16
	MaxColumns      = 64
	MaxIndexes      = 16
	MaxRowBytes     = 64 << 10
	MaxStorageBytes = 64 << 20
	MaxTableRows    = 10000
	MaxQueryRows    = 200
	MaxBatch        = 32
)

type StorageSchema struct {
	Version int            `json:"schema_version"`
	Tables  []StorageTable `json:"tables"`
}
type StorageTable struct {
	Name    string          `json:"name"`
	Columns []StorageColumn `json:"columns"`
	Indexes []StorageIndex  `json:"indexes,omitempty"`
}
type StorageColumn struct {
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	PrimaryKey bool            `json:"primary_key,omitempty"`
	Nullable   bool            `json:"nullable,omitempty"`
	Default    json.RawMessage `json:"default,omitempty"`
}
type StorageIndex struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique,omitempty"`
}
type Predicate struct {
	Column string          `json:"column"`
	Op     string          `json:"op"`
	Value  json.RawMessage `json:"value,omitempty"`
}
type Order struct {
	Column     string `json:"column"`
	Descending bool   `json:"descending,omitempty"`
}
type StorageRequest struct {
	Op      string                     `json:"op"`
	Table   string                     `json:"table"`
	Columns []string                   `json:"columns,omitempty"`
	Where   []Predicate                `json:"where,omitempty"`
	Order   []Order                    `json:"order,omitempty"`
	Limit   int                        `json:"limit,omitempty"`
	Offset  int                        `json:"offset,omitempty"`
	Values  map[string]json.RawMessage `json:"values,omitempty"`
}
type StorageResult struct {
	Rows     []map[string]any `json:"rows,omitempty"`
	Affected int64            `json:"affected"`
	ID       any              `json:"id,omitempty"`
}

var storageName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)

func ValidStorageName(value string) bool {
	return storageName.MatchString(value) && !strings.HasPrefix(value, "sqlite_") && !strings.HasPrefix(value, "cph_") && !strings.HasPrefix(value, "extension_") && value != "constructor" && value != "prototype"
}

// DecodeStrict 拒绝未知字段和多个 JSON 值，数字保留精度。
func DecodeStrict(raw []byte, target any) error {
	if len(raw) > MaxJSONBytes {
		return fmt.Errorf("JSON exceeds size limit")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

func (s StorageSchema) Validate() error {
	if s.Version < 1 || len(s.Tables) < 1 || len(s.Tables) > MaxTables {
		return fmt.Errorf("invalid storage version or table count")
	}
	tables := map[string]bool{}
	for _, t := range s.Tables {
		if !ValidStorageName(t.Name) || tables[t.Name] || len(t.Columns) < 1 || len(t.Columns) > MaxColumns || len(t.Indexes) > MaxIndexes {
			return fmt.Errorf("invalid storage table %q", t.Name)
		}
		tables[t.Name] = true
		columns := map[string]bool{}
		keys := 0
		for _, c := range t.Columns {
			if !ValidStorageName(c.Name) || columns[c.Name] {
				return fmt.Errorf("invalid storage column %q", c.Name)
			}
			columns[c.Name] = true
			switch c.Type {
			case "string", "integer", "number", "boolean", "datetime", "bytes":
			default:
				return fmt.Errorf("unsupported column type %q", c.Type)
			}
			if c.PrimaryKey {
				keys++
				if c.Nullable || len(c.Default) != 0 || (c.Type != "string" && c.Type != "integer") {
					return fmt.Errorf("invalid primary key")
				}
			}
			if len(c.Default) > 0 {
				if _, err := c.Value(c.Default); err != nil {
					return fmt.Errorf("invalid default for %s: %w", c.Name, err)
				}
			}
		}
		if keys != 1 {
			return fmt.Errorf("table %s requires exactly one primary key", t.Name)
		}
		indexes := map[string]bool{}
		for _, index := range t.Indexes {
			if !ValidStorageName(index.Name) || indexes[index.Name] || len(index.Columns) < 1 || len(index.Columns) > 4 {
				return fmt.Errorf("invalid storage index")
			}
			indexes[index.Name] = true
			seen := map[string]bool{}
			for _, name := range index.Columns {
				if !columns[name] || seen[name] {
					return fmt.Errorf("invalid index column")
				}
				seen[name] = true
			}
		}
	}
	return nil
}

// Value 将 JSON 转为规范的 SQLite 参数；整数限制在 JS 可精确表示范围内。
func (c StorageColumn) Value(raw json.RawMessage) (any, error) {
	var v any
	if err := DecodeStrict(raw, &v); err != nil {
		return nil, err
	}
	if v == nil {
		if c.Nullable {
			return nil, nil
		}
		return nil, fmt.Errorf("%s cannot be null", c.Name)
	}
	invalid := func() (any, error) { return nil, fmt.Errorf("%s requires %s", c.Name, c.Type) }
	switch c.Type {
	case "string", "datetime", "bytes":
		s, ok := v.(string)
		if !ok || len(s) > MaxRowBytes || strings.ContainsRune(s, 0) {
			return invalid()
		}
		if c.Type == "datetime" {
			value, err := time.Parse(time.RFC3339Nano, s)
			if err != nil || value.UTC().Year() < 0 || value.UTC().Year() > 9999 {
				return invalid()
			}
			// 固定小数精度使 SQLite 文本排序与时间先后相同。
			return value.UTC().Format("2006-01-02T15:04:05.000000000Z"), nil
		}
		if c.Type == "bytes" {
			value, err := base64.StdEncoding.Strict().DecodeString(s)
			if err != nil {
				return invalid()
			}
			return value, nil
		}
		return s, nil
	case "boolean":
		if value, ok := v.(bool); ok {
			if value {
				return int64(1), nil
			}
			return int64(0), nil
		}
	case "integer", "number":
		if n, ok := v.(json.Number); ok {
			if c.Type == "integer" {
				value, err := n.Int64()
				if err == nil && value >= -9007199254740991 && value <= 9007199254740991 {
					return value, nil
				}
			} else {
				value, err := n.Float64()
				if err == nil && !math.IsInf(value, 0) && !math.IsNaN(value) {
					return value, nil
				}
			}
		}
	}
	return invalid()
}

func (s StorageSchema) Canonical() ([]byte, string, error) {
	if err := s.Validate(); err != nil {
		return nil, "", err
	}
	raw, _ := json.Marshal(s)
	var copy StorageSchema
	_ = json.Unmarshal(raw, &copy)
	sort.Slice(copy.Tables, func(i, j int) bool { return copy.Tables[i].Name < copy.Tables[j].Name })
	for ti := range copy.Tables {
		t := &copy.Tables[ti]
		sort.Slice(t.Columns, func(i, j int) bool { return t.Columns[i].Name < t.Columns[j].Name })
		sort.Slice(t.Indexes, func(i, j int) bool { return t.Indexes[i].Name < t.Indexes[j].Name })
		for ci := range t.Columns {
			c := &t.Columns[ci]
			if len(c.Default) > 0 {
				value, _ := c.Value(c.Default)
				if c.Type == "boolean" && value != nil {
					value = value.(int64) != 0
				}
				c.Default, _ = json.Marshal(value)
			}
		}
	}
	raw, err := json.Marshal(copy)
	hash := sha256.Sum256(raw)
	return raw, hex.EncodeToString(hash[:]), err
}
