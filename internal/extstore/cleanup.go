package extstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

// Obsolete 只以宿主已安装的清单判断废弃表，不接受扩展提交的删除名单。
func (s *Store) Obsolete(ctx context.Context, id string, current *spec.StorageSchema, clean bool) ([]string, error) {
	if !spec.ValidID(id) {
		return nil, fmt.Errorf("invalid extension ID")
	}
	keep := map[string]bool{}
	if current != nil {
		if err := current.Validate(); err != nil {
			return nil, err
		}
		for _, table := range current.Tables {
			keep[table.Name] = true
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := []string{}
	err := s.transaction(ctx, func(tx *sql.Tx) error {
		r, err := get(ctx, tx, id)
		if err != nil || r == nil {
			return err
		}
		if !equal(r.active, current) {
			return fmt.Errorf("installed storage declaration has not been committed")
		}
		for _, name := range r.obsolete {
			if !keep[name] && r.tables[name] != "" {
				removed = append(removed, name)
			}
		}
		sort.Strings(removed)
		if !clean || len(removed) == 0 {
			return nil
		}
		for _, name := range removed {
			if session := s.sessions[id]; session != nil {
				if _, inUse := findTable(session.schema, name); inUse {
					return fmt.Errorf("table is still used by an active session")
				}
			}
			if _, err = tx.ExecContext(ctx, "DROP TABLE "+quote(r.tables[name])); err != nil {
				return err
			}
			delete(r.tables, name)
			delete(r.definitions, name)
			delete(r.committed, name)
			for version, rev := range r.history {
				for _, used := range rev.Tables {
					if used == name {
						delete(r.history, version)
						break
					}
				}
			}
		}
		used, err := recount(ctx, tx, r)
		if err != nil {
			return err
		}
		tables, _ := json.Marshal(r.tables)
		definitions, _ := json.Marshal(r.definitions)
		history, _ := json.Marshal(r.history)
		committed, _ := json.Marshal(r.committed)
		_, err = tx.ExecContext(ctx, `UPDATE extension_storage_schemas SET table_map=?,tables_json=?,history_json=?,used_bytes=?,committed_json=?,obsolete_json='[]' WHERE extension_id=?`, string(tables), string(definitions), string(history), used, string(committed), id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return removed, nil
}
