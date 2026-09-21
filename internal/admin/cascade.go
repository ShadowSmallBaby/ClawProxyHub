// cascade.go — 删除级联：插件 → 实例 → 分组/账号 → 任务规则/执行历史。
// 路由/密钥引用的分组不自动改，只列出名字提醒用户复核。
package admin

import (
	"encoding/json"
	"net/http"

	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// deleteImpact 删除影响面（预览接口与删除结果共用）。
type deleteImpact struct {
	Instances int64    `json:"instances"`
	Groups    int64    `json:"groups"`
	Accounts  int64    `json:"accounts"`
	TaskRules int64    `json:"task_rules"`
	TaskRuns  int64    `json:"task_runs"`
	Routes    []string `json:"routes"` // 引用了被删分组的路由名，需人工调整
	Keys      []string `json:"keys"`   // 授权范围含上述路由的密钥名
}

// cascadeScope 待删除对象 id 集合，由 scope* 按层级填充。
type cascadeScope struct {
	instances []int64
	groups    []int64
	accounts  []int64
	rules     []int64           // 整条删除的规则
	prune     map[int64][]int64 // 部分账号被删的 account_ids 规则 → 剩余 id
}

func pluck(db *gorm.DB, m interface{}, where string, arg interface{}) []int64 {
	var ids []int64
	db.Model(m).Where(where, arg).Pluck("id", &ids)
	return ids
}

// scopePlugin 插件下全部实例/分组/账号/规则。
func scopePlugin(db *gorm.DB, pluginID int64) cascadeScope {
	return cascadeScope{
		instances: pluck(db, &model.Instance{}, "plugin_id = ?", pluginID),
		groups:    pluck(db, &model.Group{}, "plugin_id = ?", pluginID),
		accounts:  pluck(db, &model.Account{}, "plugin_id = ?", pluginID),
		rules:     pluck(db, &model.TaskRule{}, "plugin_id = ?", pluginID),
	}
}

// scopeInstance 实例下分组/账号 + 只指向这些账号的规则。
func scopeInstance(db *gorm.DB, instanceID int64) cascadeScope {
	sc := cascadeScope{
		instances: []int64{instanceID},
		groups:    pluck(db, &model.Group{}, "instance_id = ?", instanceID),
		accounts:  pluck(db, &model.Account{}, "instance_id = ?", instanceID),
	}
	sc.rules, sc.prune = rulesByAccounts(db, sc.accounts)
	return sc
}

// scopeAccount 单账号 + 只指向它的规则。
func scopeAccount(db *gorm.DB, accountID int64) cascadeScope {
	sc := cascadeScope{accounts: []int64{accountID}}
	sc.rules, sc.prune = rulesByAccounts(db, sc.accounts)
	return sc
}

// rulesByAccounts account_ids 范围的规则：目标全在待删集合 → 整条删；部分命中 → 剔除后保留。
func rulesByAccounts(db *gorm.DB, accountIDs []int64) ([]int64, map[int64][]int64) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	gone := map[int64]bool{}
	for _, id := range accountIDs {
		gone[id] = true
	}
	var rules []model.TaskRule
	db.Where("target_scope = ?", "account_ids").Find(&rules)
	var del []int64
	prune := map[int64][]int64{}
	for _, r := range rules {
		var ids []int64
		_ = json.Unmarshal([]byte(r.TargetJSON), &ids)
		var keep []int64
		for _, id := range ids {
			if !gone[id] {
				keep = append(keep, id)
			}
		}
		switch {
		case len(keep) == len(ids):
		case len(keep) == 0:
			del = append(del, r.ID)
		default:
			prune[r.ID] = keep
		}
	}
	return del, prune
}

// runsQuery 待删账号或待删规则产生的执行历史。
func runsQuery(db *gorm.DB, sc cascadeScope) *gorm.DB {
	q := db.Model(&model.TaskRun{})
	switch {
	case len(sc.accounts) > 0 && len(sc.rules) > 0:
		return q.Where("account_id IN ? OR rule_id IN ?", sc.accounts, sc.rules)
	case len(sc.accounts) > 0:
		return q.Where("account_id IN ?", sc.accounts)
	case len(sc.rules) > 0:
		return q.Where("rule_id IN ?", sc.rules)
	}
	return q.Where("1 = 0")
}

// impact 统计影响面：数量 + 引用被删分组的路由/密钥名。
func (s *Server) impact(sc cascadeScope) deleteImpact {
	out := deleteImpact{
		Instances: int64(len(sc.instances)), Groups: int64(len(sc.groups)),
		Accounts: int64(len(sc.accounts)), TaskRules: int64(len(sc.rules)),
		Routes: []string{}, Keys: []string{},
	}
	runsQuery(s.db, sc).Count(&out.TaskRuns)
	if len(sc.groups) == 0 {
		return out
	}
	gone := map[int64]bool{}
	for _, id := range sc.groups {
		gone[id] = true
	}
	var routes []model.Route
	s.db.Order("id").Find(&routes)
	var routeIDs []int64
	for _, rt := range routes {
		hit := rt.FailoverGroupID != nil && gone[*rt.FailoverGroupID]
		var entries []model.RouteGroupEntry
		_ = json.Unmarshal([]byte(rt.GroupsJSON), &entries)
		for _, e := range entries {
			hit = hit || gone[e.GroupID]
		}
		if hit {
			routeIDs = append(routeIDs, rt.ID)
			out.Routes = append(out.Routes, rt.Name)
		}
	}
	if len(routeIDs) > 0 {
		s.db.Model(&model.Key{}).Distinct("keys.name").
			Joins("JOIN key_routes ON key_routes.key_id = keys.id").
			Where("key_routes.route_id IN ?", routeIDs).Order("keys.name").Pluck("keys.name", &out.Keys)
	}
	return out
}

// cascadeDelete 事务内按层级删除；插件记录本身由调用方处理。
func (s *Server) cascadeDelete(sc cascadeScope) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := runsQuery(tx, sc).Delete(&model.TaskRun{}).Error; err != nil {
			return err
		}
		for id, keep := range sc.prune {
			b, _ := json.Marshal(keep)
			tx.Model(&model.TaskRule{}).Where("id = ?", id).Update("target_json", string(b))
		}
		if len(sc.rules) > 0 {
			tx.Where("id IN ?", sc.rules).Delete(&model.TaskRule{})
		}
		if len(sc.accounts) > 0 {
			tx.Where("account_id IN ?", sc.accounts).Delete(&model.AccountGroup{})
			tx.Where("account_id IN ?", sc.accounts).Delete(&model.AccountProxy{})
			tx.Where("id IN ?", sc.accounts).Delete(&model.Account{})
		}
		if len(sc.groups) > 0 {
			tx.Where("group_id IN ?", sc.groups).Delete(&model.GroupProxy{})
			tx.Where("id IN ?", sc.groups).Delete(&model.Group{})
		}
		if len(sc.instances) > 0 {
			tx.Where("id IN ?", sc.instances).Delete(&model.Instance{})
		}
		return tx.Error
	})
}

// pluginImpact GET /admin/plugins/{name}/impact — 卸载预览。
func (s *Server) pluginImpact(w http.ResponseWriter, r *http.Request) {
	var p model.Plugin
	if err := s.db.Where("name = ?", r.PathValue("name")).First(&p).Error; err != nil {
		writeJSON(w, http.StatusOK, s.impact(cascadeScope{}))
		return
	}
	writeJSON(w, http.StatusOK, s.impact(scopePlugin(s.db, p.ID)))
}

// instanceImpact GET /admin/instances/{id}/impact — 删除预览。
func (s *Server) instanceImpact(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.impact(scopeInstance(s.db, parseInt(r.PathValue("id")))))
}

// accountImpact GET /admin/accounts/{id}/impact — 删除预览。
func (s *Server) accountImpact(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.impact(scopeAccount(s.db, parseInt(r.PathValue("id")))))
}
