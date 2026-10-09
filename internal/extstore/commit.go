package extstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
)

// SyncInstalled 仅按已提交的安装状态改变废弃标记，失败候选的表不会成为历史业务表。
func (s *Store) SyncInstalled(ctx context.Context, states []Installed) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	installed := map[string]Installed{}
	for _, state := range states {
		installed[state.ID] = state
	}
	return s.transaction(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT extension_id FROM extension_storage_schemas`)
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
		for _, id := range ids {
			r, err := get(ctx, tx, id)
			if err != nil {
				return err
			}
			state, exists := installed[id]
			keep := map[string]bool{}
			phase := "retained"
			installedHash := ""
			if exists {
				installedHash = state.Hash
				r.active = state.Storage
				phase = "ready"
				if state.Storage != nil {
					_, hash, err := state.Storage.Canonical()
					if err != nil {
						return err
					}
					version := strconv.Itoa(state.Storage.Version)
					rev := r.history[version]
					if rev.Hash == hash {
						rev.Committed = true
						r.history[version] = rev
					} else {
						phase = "pending"
					}
					for _, table := range state.Storage.Tables {
						keep[table.Name] = true
						if _, present := r.definitions[table.Name]; present {
							r.committed[table.Name] = true
						}
					}
				}
			} else if r.active != nil {
				for _, table := range r.active.Tables {
					keep[table.Name] = true
				}
			}
			changed := false
			for name := range r.tables {
				if r.committed[name] || keep[name] {
					continue
				}
				if session := s.sessions[id]; session != nil {
					if _, inUse := findTable(session.schema, name); inUse {
						continue
					}
				}
				if _, err = tx.ExecContext(ctx, "DROP TABLE "+quote(r.tables[name])); err != nil {
					return err
				}
				delete(r.tables, name)
				delete(r.definitions, name)
				changed = true
			}
			if changed {
				r.used, err = recount(ctx, tx, r)
				if err != nil {
					return err
				}
			}
			if exists {
				r.obsolete = []string{}
				for name := range r.tables {
					if r.committed[name] && !keep[name] {
						r.obsolete = append(r.obsolete, name)
					}
				}
				sort.Strings(r.obsolete)
			}
			tables, _ := json.Marshal(r.tables)
			definitions, _ := json.Marshal(r.definitions)
			history, _ := json.Marshal(r.history)
			committed, _ := json.Marshal(r.committed)
			active, _ := json.Marshal(r.active)
			obsolete, _ := json.Marshal(r.obsolete)
			_, err = tx.ExecContext(ctx, `UPDATE extension_storage_schemas SET table_map=?,tables_json=?,history_json=?,used_bytes=?,installed_package=?,phase=?,committed_json=?,active_schema=?,obsolete_json=? WHERE extension_id=?`, string(tables), string(definitions), string(history), r.used, installedHash, phase, string(committed), string(active), string(obsolete), id)
			if err != nil {
				return err
			}
		}
		return nil
	})
}
