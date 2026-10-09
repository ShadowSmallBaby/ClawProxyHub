package extstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
	sqlite "github.com/mattn/go-sqlite3"
)

func findTable(schema spec.StorageSchema, name string) (spec.StorageTable, bool) {
	for _, table := range schema.Tables {
		if table.Name == name {
			return table, true
		}
	}
	return spec.StorageTable{}, false
}
func columnsOf(table spec.StorageTable) map[string]spec.StorageColumn {
	columns := map[string]spec.StorageColumn{}
	for _, column := range table.Columns {
		columns[column.Name] = column
	}
	return columns
}
func keyOf(table spec.StorageTable) spec.StorageColumn {
	for _, column := range table.Columns {
		if column.PrimaryKey {
			return column
		}
	}
	panic("validated primary key required")
}
func columnNames(table spec.StorageTable) []string {
	names := []string{}
	for _, c := range table.Columns {
		names = append(names, c.Name)
	}
	return names
}
func selection(names []string) string {
	result := []string{}
	for _, name := range names {
		result = append(result, quote(name))
	}
	return strings.Join(result, ",")
}

func predicates(table spec.StorageTable, where []spec.Predicate) (string, []any, error) {
	if len(where) > 16 {
		return "", nil, fmt.Errorf("too many predicates")
	}
	columns := columnsOf(table)
	parts := []string{}
	args := []any{}
	for _, condition := range where {
		column, ok := columns[condition.Column]
		if !ok {
			return "", nil, fmt.Errorf("undeclared filter column")
		}
		name := quote(column.Name)
		if condition.Op == "is_null" || condition.Op == "not_null" {
			if len(condition.Value) > 0 {
				return "", nil, fmt.Errorf("null predicates do not take a value")
			}
			operator := " IS NULL"
			if condition.Op == "not_null" {
				operator = " IS NOT NULL"
			}
			parts = append(parts, name+operator)
			continue
		}
		if condition.Op == "in" {
			var values []json.RawMessage
			if err := spec.DecodeStrict(condition.Value, &values); err != nil || len(values) < 1 || len(values) > 100 {
				return "", nil, fmt.Errorf("in requires 1..100 values")
			}
			marks := []string{}
			for _, raw := range values {
				value, err := column.Value(raw)
				if err != nil || value == nil {
					return "", nil, fmt.Errorf("invalid in value")
				}
				args = append(args, value)
				marks = append(marks, "?")
			}
			parts = append(parts, name+" IN ("+strings.Join(marks, ",")+")")
			continue
		}
		operator := map[string]string{"eq": "=", "ne": "<>", "lt": "<", "lte": "<=", "gt": ">", "gte": ">="}[condition.Op]
		if operator == "" {
			return "", nil, fmt.Errorf("unsupported filter operator")
		}
		value, err := column.Value(condition.Value)
		if err != nil {
			return "", nil, err
		}
		if value == nil {
			if condition.Op == "eq" {
				parts = append(parts, name+" IS NULL")
			} else if condition.Op == "ne" {
				parts = append(parts, name+" IS NOT NULL")
			} else {
				return "", nil, fmt.Errorf("null only supports equality")
			}
		} else {
			parts = append(parts, name+operator+"?")
			args = append(args, value)
		}
	}
	if len(parts) == 0 {
		return "", args, nil
	}
	return " WHERE " + strings.Join(parts, " AND "), args, nil
}

type storedRow struct {
	values map[string]any
	size   int64
}

func readRows(ctx context.Context, tx *sql.Tx, query string, args []any, table spec.StorageTable, names []string, withSize bool) ([]storedRow, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := columnsOf(table)
	result := []storedRow{}
	for rows.Next() {
		count := len(names)
		if withSize {
			count++
		}
		values := make([]any, count)
		pointers := make([]any, count)
		for i := range values {
			pointers[i] = &values[i]
		}
		if err = rows.Scan(pointers...); err != nil {
			return nil, err
		}
		row := storedRow{values: map[string]any{}}
		for i, name := range names {
			value := values[i]
			if value != nil && columns[name].Type == "boolean" {
				n, ok := value.(int64)
				if !ok || (n != 0 && n != 1) {
					return nil, fmt.Errorf("invalid stored boolean")
				}
				value = n == 1
			}
			row.values[name] = value
		}
		if withSize {
			size, ok := values[len(names)].(int64)
			if !ok || size < 0 {
				return nil, fmt.Errorf("invalid stored row size")
			}
			row.size = size
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
func rowSize(row map[string]any) (int64, error) {
	raw, err := json.Marshal(row)
	if err != nil {
		return 0, err
	}
	if len(raw) > spec.MaxRowBytes {
		return 0, fmt.Errorf("row exceeds storage size limit")
	}
	return int64(len(raw)), nil
}

func recount(ctx context.Context, tx *sql.Tx, r *record) (int64, error) {
	var total int64
	for _, table := range r.definitions {
		names := columnNames(table)
		key := keyOf(table)
		physical := quote(r.tables[table.Name])
		offset := 0
		for {
			rows, err := readRows(ctx, tx, "SELECT "+selection(names)+",\"_cph_size\" FROM "+physical+" ORDER BY "+quote(key.Name)+" LIMIT ? OFFSET ?", []any{spec.MaxQueryRows, offset}, table, names, true)
			if err != nil {
				return 0, err
			}
			for _, row := range rows {
				size, err := rowSize(row.values)
				if err != nil {
					return 0, err
				}
				total += size
				if _, err = tx.ExecContext(ctx, "UPDATE "+physical+" SET \"_cph_size\"=? WHERE "+quote(key.Name)+"=?", size, row.values[key.Name]); err != nil {
					return 0, err
				}
			}
			offset += len(rows)
			if offset > spec.MaxTableRows {
				return 0, fmt.Errorf("table exceeds row limit")
			}
			if len(rows) < spec.MaxQueryRows {
				break
			}
		}
	}
	return total, nil
}

// Execute 的身份来自会话，权限由宿主当前调用上下文传入；扩展无法选择其他所有者。
func (v *Session) Execute(ctx context.Context, requests []spec.StorageRequest, grants []string) ([]spec.StorageResult, error) {
	if len(requests) < 1 || len(requests) > spec.MaxBatch {
		return nil, fmt.Errorf("storage batch requires 1..%d requests", spec.MaxBatch)
	}
	raw, err := json.Marshal(requests)
	if err != nil || len(raw) > spec.MaxJSONBytes {
		return nil, fmt.Errorf("storage request too large")
	}
	if v.ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(v.ctx, cancel)
	defer stop()
	results := []spec.StorageResult{}
	err = v.store.transaction(ctx, func(tx *sql.Tx) error {
		if v.ctx.Err() != nil {
			return ErrUnavailable
		}
		r, err := get(ctx, tx, v.id)
		if err != nil {
			return err
		}
		if r == nil {
			return ErrUnavailable
		}
		for _, request := range requests {
			permission := "storage.write"
			if request.Op == "query" {
				permission = "storage.read"
			}
			if !spec.HasPermission(grants, permission) {
				return fmt.Errorf("storage permission denied: %s", permission)
			}
			table, ok := findTable(v.schema, request.Table)
			if !ok {
				return fmt.Errorf("undeclared storage table")
			}
			current, ok := r.definitions[request.Table]
			if !ok || r.tables[request.Table] != v.tables[request.Table] {
				return ErrUnavailable
			}
			result, err := operate(ctx, tx, r, table, current, request)
			if err != nil {
				return err
			}
			results = append(results, result)
			encoded, err := json.Marshal(results)
			if err != nil || len(encoded) > spec.MaxJSONBytes {
				return fmt.Errorf("storage response too large")
			}
		}
		return nil
	})
	if err != nil {
		var dbError sqlite.Error
		if errors.As(err, &dbError) {
			if dbError.Code == sqlite.ErrConstraint {
				return nil, fmt.Errorf("storage constraint violation")
			}
			if dbError.Code == sqlite.ErrBusy || dbError.Code == sqlite.ErrLocked {
				return nil, ErrBusy
			}
			if dbError.Code == sqlite.ErrFull {
				return nil, fmt.Errorf("shared storage capacity exceeded")
			}
			return nil, fmt.Errorf("storage database operation failed")
		}
		return nil, err
	}
	return results, nil
}

func operate(ctx context.Context, tx *sql.Tx, r *record, table, current spec.StorageTable, request spec.StorageRequest) (spec.StorageResult, error) {
	result := spec.StorageResult{}
	name := quote(r.tables[table.Name])
	key := keyOf(table)
	allowed := columnsOf(table)
	where, args, err := predicates(table, request.Where)
	if err != nil {
		return result, err
	}
	if request.Op == "query" {
		if len(request.Values) > 0 || request.Limit < 0 || request.Limit > spec.MaxQueryRows || request.Offset < 0 || request.Offset > spec.MaxTableRows || len(request.Order) > 4 {
			return result, fmt.Errorf("invalid query bounds")
		}
		names := request.Columns
		if len(names) == 0 {
			names = columnNames(table)
		}
		seen := map[string]bool{}
		for _, n := range names {
			if _, ok := allowed[n]; !ok || seen[n] {
				return result, fmt.Errorf("undeclared or duplicate selected column")
			}
			seen[n] = true
		}
		order := []string{}
		seen = map[string]bool{}
		for _, entry := range request.Order {
			if _, ok := allowed[entry.Column]; !ok || seen[entry.Column] {
				return result, fmt.Errorf("undeclared or duplicate order column")
			}
			seen[entry.Column] = true
			direction := " ASC"
			if entry.Descending {
				direction = " DESC"
			}
			order = append(order, quote(entry.Column)+direction)
		}
		if !seen[key.Name] {
			order = append(order, quote(key.Name)+" ASC")
		}
		limit := request.Limit
		if limit == 0 {
			limit = 50
		}
		args = append(args, limit, request.Offset)
		rows, err := readRows(ctx, tx, "SELECT "+selection(names)+" FROM "+name+where+" ORDER BY "+strings.Join(order, ",")+" LIMIT ? OFFSET ?", args, table, names, false)
		if err != nil {
			return result, err
		}
		result.Rows = []map[string]any{}
		for _, row := range rows {
			result.Rows = append(result.Rows, row.values)
		}
		return result, nil
	}
	if len(request.Columns) > 0 || len(request.Order) > 0 || request.Limit != 0 || request.Offset != 0 {
		return result, fmt.Errorf("query options are not allowed in mutations")
	}
	values := map[string]any{}
	for field, raw := range request.Values {
		column, ok := allowed[field]
		if !ok {
			return result, fmt.Errorf("undeclared value column")
		}
		if column.PrimaryKey && request.Op == "update" {
			return result, fmt.Errorf("primary key cannot be changed")
		}
		value, err := column.Value(raw)
		if err != nil {
			return result, err
		}
		values[field] = value
	}
	switch request.Op {
	case "insert":
		if len(request.Where) > 0 {
			return result, fmt.Errorf("insert does not take filters")
		}
		var count int64
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM "+name).Scan(&count); err != nil {
			return result, err
		}
		if count >= spec.MaxTableRows {
			return result, fmt.Errorf("table row quota exceeded")
		}
		for _, column := range table.Columns {
			if _, exists := values[column.Name]; exists {
				continue
			}
			if column.PrimaryKey {
				if column.Type == "string" {
					value := make([]byte, 16)
					if _, err = rand.Read(value); err != nil {
						return result, err
					}
					values[column.Name] = hex.EncodeToString(value)
				}
				continue
			}
			if len(column.Default) > 0 {
				values[column.Name], _ = column.Value(column.Default)
			} else if column.Nullable {
				values[column.Name] = nil
			} else {
				return result, fmt.Errorf("missing column %s", column.Name)
			}
		}
		names := []string{}
		marks := []string{}
		args = []any{}
		for _, column := range table.Columns {
			if value, exists := values[column.Name]; exists {
				names = append(names, column.Name)
				marks = append(marks, "?")
				args = append(args, value)
			}
		}
		statement := "INSERT INTO " + name + " DEFAULT VALUES"
		if len(names) > 0 {
			statement = "INSERT INTO " + name + " (" + selection(names) + ") VALUES (" + strings.Join(marks, ",") + ")"
		}
		inserted, err := tx.ExecContext(ctx, statement, args...)
		if err != nil {
			return result, err
		}
		id := values[key.Name]
		if id == nil {
			generated, err := inserted.LastInsertId()
			if err != nil {
				return result, err
			}
			if generated > 9007199254740991 {
				return result, fmt.Errorf("generated integer exceeds supported range")
			}
			id = generated
		}
		names = columnNames(current)
		rows, err := readRows(ctx, tx, "SELECT "+selection(names)+",\"_cph_size\" FROM "+name+" WHERE "+quote(key.Name)+"=?", []any{id}, current, names, true)
		if err != nil {
			return result, err
		}
		if len(rows) != 1 {
			return result, fmt.Errorf("inserted row unavailable")
		}
		size, err := rowSize(rows[0].values)
		if err != nil {
			return result, err
		}
		if err = addUsage(ctx, tx, r, size); err != nil {
			return result, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE "+name+" SET \"_cph_size\"=? WHERE "+quote(key.Name)+"=?", size, id); err != nil {
			return result, err
		}
		result.ID = id
		result.Affected = 1
		return result, nil
	case "update", "delete":
		if len(request.Where) == 0 {
			return result, fmt.Errorf("mutation requires a filter")
		}
		if request.Op == "delete" && len(values) > 0 || request.Op == "update" && len(values) == 0 {
			return result, fmt.Errorf("invalid mutation values")
		}
		names := columnNames(current)
		queryArgs := append(append([]any{}, args...), spec.MaxQueryRows+1)
		rows, err := readRows(ctx, tx, "SELECT "+selection(names)+",\"_cph_size\" FROM "+name+where+" ORDER BY "+quote(key.Name)+" LIMIT ?", queryArgs, current, names, true)
		if err != nil {
			return result, err
		}
		if len(rows) > spec.MaxQueryRows {
			return result, fmt.Errorf("mutation affects too many rows")
		}
		for _, row := range rows {
			id := row.values[key.Name]
			if request.Op == "delete" {
				if _, err = tx.ExecContext(ctx, "DELETE FROM "+name+" WHERE "+quote(key.Name)+"=?", id); err != nil {
					return result, err
				}
				if err = addUsage(ctx, tx, r, -row.size); err != nil {
					return result, err
				}
			} else {
				sets := []string{}
				args = []any{}
				for _, column := range table.Columns {
					if value, exists := values[column.Name]; exists {
						sets = append(sets, quote(column.Name)+"=?")
						args = append(args, value)
						display := value
						if column.Type == "boolean" && value != nil {
							display = value.(int64) != 0
						}
						row.values[column.Name] = display
					}
				}
				size, err := rowSize(row.values)
				if err != nil {
					return result, err
				}
				if err = addUsage(ctx, tx, r, size-row.size); err != nil {
					return result, err
				}
				sets = append(sets, `"_cph_size"=?`)
				args = append(args, size, id)
				if _, err = tx.ExecContext(ctx, "UPDATE "+name+" SET "+strings.Join(sets, ",")+" WHERE "+quote(key.Name)+"=?", args...); err != nil {
					return result, err
				}
			}
			result.Affected++
		}
		return result, nil
	default:
		return result, fmt.Errorf("unsupported storage operation")
	}
}
func addUsage(ctx context.Context, tx *sql.Tx, r *record, delta int64) error {
	r.used += delta
	if r.used < 0 || r.used > spec.MaxStorageBytes {
		return fmt.Errorf("extension storage quota exceeded")
	}
	_, err := tx.ExecContext(ctx, `UPDATE extension_storage_schemas SET used_bytes=?,updated_at=? WHERE extension_id=?`, r.used, time.Now().UTC().Format(time.RFC3339Nano), r.id)
	return err
}
