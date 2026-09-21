// Package router — 路由解析：对外模型名（路由）→ 分组（权重+真实模型）→ 账号（策略）。
// key 授权路由为空 = 全部路由。会话粘性：首轮消息指纹钉住分组+账号。
package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

const stickyTTL = 30 * time.Minute

// stickyEntry 粘性缓存条目。
type stickyEntry struct {
	entry     model.RouteGroupEntry
	accountID int64
	expires   time.Time
}

// Router 路由解析器。
type Router struct {
	db     *gorm.DB
	mu     sync.Mutex
	rr     map[int64]int64        // routeID → 轮询计数
	sticky map[string]stickyEntry // 指纹 → 分组+账号
}

func New(db *gorm.DB) *Router {
	return &Router{db: db, rr: map[int64]int64{}, sticky: map[string]stickyEntry{}}
}

// Resolved 路由解析结果。
type Resolved struct {
	Route      *model.Route
	Account    *model.Account // nil = 分组内无账号，凭据为空
	RealModel  string         // 分组对应的真实模型 id
	GroupID    int64          // 命中的分组（凭据注入出站代理时用）
	PluginName string         // 分组所属插件（真实模型不必在插件目录中，直接透传上游）
}

// Resolve 按对外模型名解析路由。
// (nil, nil) = 不是路由名；ErrRouteForbidden = 路由存在但 key 无权；正常返回解析结果。
func (r *Router) Resolve(key *model.Key, req *pb.ChatRequest) (*Resolved, error) {
	if key == nil {
		return nil, nil
	}
	route, err := r.findRoute(key, req.Model)
	if err != nil || route == nil {
		return nil, err
	}
	entries, err := parseGroups(route.GroupsJSON)
	if err != nil || len(entries) == 0 {
		return nil, err
	}

	// 粘性优先：指纹命中且账号可用则复用
	fp := Fingerprint(req)
	if route.Strategy == "sticky" {
		if res := r.lookupSticky(fp, route, entries); res != nil {
			r.markUsed(res.Account.ID)
			return res, nil
		}
	}

	// 按权重选分组
	entry := pickGroup(entries)

	var acct *model.Account
	switch route.Strategy {
	case "random":
		acct = r.byRandom(entry.GroupID)
	case "least_used":
		acct = r.byLeastUsed(entry.GroupID)
	default: // round_robin / sticky（未命中退化为轮询）
		acct = r.byRoundRobin(route.ID, entry.GroupID)
	}

	if acct != nil && route.Strategy == "sticky" {
		r.saveSticky(fp, entry, acct.ID)
	}
	if acct != nil {
		r.markUsed(acct.ID)
	}
	return &Resolved{Route: route, Account: acct, RealModel: entry.Model, GroupID: entry.GroupID, PluginName: r.groupPlugin(entry.GroupID)}, nil
}

// PickFailover 解析路由的降级目标：降级分组内按路由策略选账号。
// 配置不全或分组无可用账号时返回 nil。不写粘性缓存（降级是临时切换）。
func (r *Router) PickFailover(route *model.Route) *Resolved {
	if route == nil || route.FailoverGroupID == nil || *route.FailoverGroupID == 0 || route.FailoverModel == "" {
		return nil
	}
	gid := *route.FailoverGroupID
	var acct *model.Account
	switch route.Strategy {
	case "random":
		acct = r.byRandom(gid)
	case "least_used":
		acct = r.byLeastUsed(gid)
	default:
		acct = r.byRoundRobin(route.ID, gid)
	}
	if acct == nil {
		return nil
	}
	r.markUsed(acct.ID)
	return &Resolved{Route: route, Account: acct, RealModel: route.FailoverModel, GroupID: gid, PluginName: r.groupPlugin(gid)}
}

// groupPlugin 分组所属插件名。
func (r *Router) groupPlugin(groupID int64) string {
	var g model.Group
	if err := r.db.Select("plugin_id").First(&g, groupID).Error; err != nil {
		return ""
	}
	var p model.Plugin
	if err := r.db.Select("name").First(&p, g.PluginID).Error; err != nil {
		return ""
	}
	return p.Name
}

// ErrRouteForbidden 路由存在但 key 未授权（网关应回 403 而非 404）。
var ErrRouteForbidden = fmt.Errorf("route exists but key is not authorized for it")

// findRoute 按对外名查路由并校验 key 授权范围。
// (nil, nil) = 不是路由名；(nil, ErrRouteForbidden) = 路由存在但无权。
func (r *Router) findRoute(key *model.Key, name string) (*model.Route, error) {
	var route model.Route
	if err := r.db.Where("name = ?", name).First(&route).Error; err != nil {
		return nil, nil // 不是路由名，走真实模型名 fallback
	}
	// key 绑定了授权范围则必须在范围内
	var count int64
	r.db.Model(&model.KeyRoute{}).Where("key_id = ?", key.ID).Count(&count)
	if count > 0 {
		r.db.Model(&model.KeyRoute{}).
			Where("key_id = ? AND route_id = ?", key.ID, route.ID).Count(&count)
		if count == 0 {
			return nil, ErrRouteForbidden
		}
	}
	return &route, nil
}

// AuthorizedModels key 授权的对外模型名列表。
// 未绑范围 = 全部路由名；无任何路由时返回 nil（调用方 fallback 插件目录）。
func (r *Router) AuthorizedModels(key *model.Key) []string {
	var names []string
	var count int64
	r.db.Model(&model.KeyRoute{}).Where("key_id = ?", key.ID).Count(&count)
	if count == 0 {
		r.db.Model(&model.Route{}).Order("name").Pluck("name", &names)
		return names
	}
	r.db.Raw(`SELECT r.name FROM routes r
	    JOIN key_routes kr ON kr.route_id = r.id
	    WHERE kr.key_id = ? ORDER BY r.name`, key.ID).Scan(&names)
	return names
}

// activeWhere 选号的可用性条件：active 且不在自动暂停期（429 限速等，到期自动恢复）。
func (r *Router) activeWhere(db *gorm.DB) *gorm.DB {
	return db.Where("status = ? AND (paused_until IS NULL OR paused_until < ?)", "active", time.Now())
}

// accountsInGroup 分组内全部可用账号（经 account_groups 多对多）。
func (r *Router) accountsInGroup(groupID int64) []model.Account {
	var accts []model.Account
	if err := r.activeWhere(r.db).
		Joins("JOIN account_groups ag ON ag.account_id = accounts.id").
		Where("ag.group_id = ?", groupID).Find(&accts).Error; err != nil {
		return nil
	}
	return accts
}

// byRoundRobin 分组内轮询。
func (r *Router) byRoundRobin(routeID, groupID int64) *model.Account {
	accts := r.accountsInGroup(groupID)
	if len(accts) == 0 {
		return nil
	}
	r.mu.Lock()
	r.rr[routeID]++
	idx := r.rr[routeID]
	r.mu.Unlock()
	return &accts[idx%int64(len(accts))]
}

// byRandom 分组内随机。
func (r *Router) byRandom(groupID int64) *model.Account {
	accts := r.accountsInGroup(groupID)
	if len(accts) == 0 {
		return nil
	}
	return &accts[rand.Intn(len(accts))]
}

// byLeastUsed 分组内最少使用优先（空闲账号优先吃新会话）。
func (r *Router) byLeastUsed(groupID int64) *model.Account {
	var acct model.Account
	err := r.activeWhere(r.db).
		Joins("JOIN account_groups ag ON ag.account_id = accounts.id").
		Where("ag.group_id = ?", groupID).
		Order("last_used_at IS NULL DESC, last_used_at").First(&acct).Error
	if err != nil {
		return nil
	}
	return &acct
}

// lookupSticky 查粘性缓存，账号失效或不在路由分组内则丢弃。
func (r *Router) lookupSticky(fp string, route *model.Route, entries []model.RouteGroupEntry) *Resolved {
	r.mu.Lock()
	e, ok := r.sticky[fp]
	if ok && time.Now().After(e.expires) {
		delete(r.sticky, fp)
		ok = false
	}
	r.mu.Unlock()
	if !ok {
		return nil
	}
	// 缓存的分组条目必须仍在路由内（路由改配置后失效）
	valid := false
	for _, en := range entries {
		if en.GroupID == e.entry.GroupID && en.Model == e.entry.Model {
			valid = true
			break
		}
	}
	if !valid {
		return nil
	}
	var acct model.Account
	if err := r.activeWhere(r.db).Where("id = ?", e.accountID).First(&acct).Error; err != nil {
		return nil
	}
	// 账号必须仍在缓存命中的分组内
	var n int64
	r.db.Model(&model.AccountGroup{}).
		Where("account_id = ? AND group_id = ?", e.accountID, e.entry.GroupID).Count(&n)
	if n == 0 {
		return nil
	}
	entry := e.entry
	return &Resolved{Route: route, Account: &acct, RealModel: entry.Model, GroupID: entry.GroupID, PluginName: r.groupPlugin(entry.GroupID)}
}

func (r *Router) saveSticky(fp string, entry model.RouteGroupEntry, accountID int64) {
	r.mu.Lock()
	r.sticky[fp] = stickyEntry{entry: entry, accountID: accountID, expires: time.Now().Add(stickyTTL)}
	// 顺手清理过期项，避免无限增长
	if len(r.sticky) > 4096 {
		now := time.Now()
		for k, v := range r.sticky {
			if now.After(v.expires) {
				delete(r.sticky, k)
			}
		}
	}
	r.mu.Unlock()
}

// markUsed 更新账号使用时间。
func (r *Router) markUsed(accountID int64) {
	r.db.Model(&model.Account{}).Where("id = ?", accountID).
		Update("last_used_at", time.Now())
}

// Fingerprint 会话指纹：哈希首轮对话开头（到第一条 user 消息为止）。
// 多轮对话里前缀稳定，同一会话的后续请求能命中同一账号。
func Fingerprint(req *pb.ChatRequest) string {
	h := sha256.New()
	for _, m := range req.Messages {
		fmt.Fprintf(h, "%s\x00%s\x00", m.Role, m.Text)
		if m.Role == "user" {
			break
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// pickGroup 按权重随机选分组：权重 0 = 不参与（如 100+0+0 只走第一个）；全为 0 时退回第一个。
func pickGroup(entries []model.RouteGroupEntry) model.RouteGroupEntry {
	total := 0
	for _, e := range entries {
		if e.Weight > 0 {
			total += e.Weight
		}
	}
	if total == 0 {
		return entries[0]
	}
	n := rand.Intn(total)
	for _, e := range entries {
		if e.Weight <= 0 {
			continue
		}
		n -= e.Weight
		if n < 0 {
			return e
		}
	}
	return entries[len(entries)-1]
}

func parseGroups(raw string) ([]model.RouteGroupEntry, error) {
	var entries []model.RouteGroupEntry
	err := json.Unmarshal([]byte(raw), &entries)
	return entries, err
}
