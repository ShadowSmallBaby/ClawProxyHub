package admin

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/database"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// seedCascade 建一套 插件 → 2 实例 → 分组/账号 → 规则/执行 → 路由/密钥 的数据。
func seedCascade(t *testing.T) (*gorm.DB, map[string]int64) {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	// Windows 下 TempDir 清理前须关连接
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	p := model.Plugin{Name: "p", Version: "1", Author: "a", ManifestJSON: "{}"}
	db.Create(&p)
	i1 := model.Instance{PluginID: p.ID, Name: "i1"}
	i2 := model.Instance{PluginID: p.ID, Name: "i2"}
	db.Create(&i1)
	db.Create(&i2)
	a1 := model.Account{PluginID: p.ID, InstanceID: i1.ID, DisplayName: "a1"}
	a2 := model.Account{PluginID: p.ID, InstanceID: i1.ID, DisplayName: "a2"}
	a3 := model.Account{PluginID: p.ID, InstanceID: i2.ID, DisplayName: "a3"}
	db.Create(&a1)
	db.Create(&a2)
	db.Create(&a3)
	g1 := model.Group{Name: "g1", PluginID: p.ID, InstanceID: i1.ID}
	g2 := model.Group{Name: "g2", PluginID: p.ID, InstanceID: i2.ID}
	db.Create(&g1)
	db.Create(&g2)
	db.Create(&model.AccountGroup{AccountID: a1.ID, GroupID: g1.ID})
	// r1 只指向 a1（删 a1 整条删）；r2 指向 a1+a3（删 a1 剔除保留）；r3 全部账号
	r1 := model.TaskRule{PluginID: p.ID, CapabilityID: "c", TriggerType: "daily", TriggerValue: "09:00", TargetScope: "account_ids", TargetJSON: `[` + itoa(a1.ID) + `]`}
	r2 := model.TaskRule{PluginID: p.ID, CapabilityID: "c", TriggerType: "daily", TriggerValue: "09:00", TargetScope: "account_ids", TargetJSON: `[` + itoa(a1.ID) + `,` + itoa(a3.ID) + `]`}
	r3 := model.TaskRule{PluginID: p.ID, CapabilityID: "c", TriggerType: "daily", TriggerValue: "09:00", TargetScope: "all", TargetJSON: "[]"}
	db.Create(&r1)
	db.Create(&r2)
	db.Create(&r3)
	db.Create(&model.TaskRun{RuleID: &r1.ID, AccountID: &a1.ID, Status: "success"})
	db.Create(&model.TaskRun{RuleID: &r3.ID, AccountID: &a3.ID, Status: "success"})
	// 路由 rt1 引用 g1；rt2 只用 g2 作降级；密钥 k1 只授权 rt1，k2 授权全部（不受影响）
	rt1 := model.Route{Name: "rt1", GroupsJSON: `[{"group_id":` + itoa(g1.ID) + `,"weight":100,"model":"m"}]`}
	rt2 := model.Route{Name: "rt2", GroupsJSON: `[]`, FailoverGroupID: &g2.ID}
	db.Create(&rt1)
	db.Create(&rt2)
	k1 := model.Key{KeyCipher: "k1", Name: "k1"}
	k2 := model.Key{KeyCipher: "k2", Name: "k2"}
	db.Create(&k1)
	db.Create(&k2)
	db.Create(&model.KeyRoute{KeyID: k1.ID, RouteID: rt1.ID})
	return db, map[string]int64{"p": p.ID, "i1": i1.ID, "i2": i2.ID, "a1": a1.ID, "r1": r1.ID, "r2": r2.ID, "r3": r3.ID, "g1": g1.ID}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func count(db *gorm.DB, m interface{}) int64 {
	var n int64
	db.Model(m).Count(&n)
	return n
}

func TestCascadeAccount(t *testing.T) {
	db, ids := seedCascade(t)
	s := &Server{db: db}
	sc := scopeAccount(db, ids["a1"])
	im := s.impact(sc)
	if im.TaskRules != 1 || im.TaskRuns != 1 || im.Groups != 0 || len(im.Routes) != 0 {
		t.Fatalf("unexpected impact: %+v", im)
	}
	if err := s.cascadeDelete(sc); err != nil {
		t.Fatal(err)
	}
	if count(db, &model.Account{}) != 2 || count(db, &model.TaskRule{}) != 2 || count(db, &model.TaskRun{}) != 1 {
		t.Fatalf("cascade mismatch: accounts=%d rules=%d runs=%d", count(db, &model.Account{}), count(db, &model.TaskRule{}), count(db, &model.TaskRun{}))
	}
	var r2 model.TaskRule
	db.First(&r2, ids["r2"])
	if r2.TargetJSON != "["+itoa(ids["a1"]+2)+"]" {
		t.Fatalf("r2 should be pruned to a3 only, got %s", r2.TargetJSON)
	}
}

func TestCascadeInstanceRoutesHint(t *testing.T) {
	db, ids := seedCascade(t)
	s := &Server{db: db}
	sc := scopeInstance(db, ids["i1"])
	im := s.impact(sc)
	if im.Accounts != 2 || im.Groups != 1 || im.TaskRules != 1 {
		t.Fatalf("unexpected impact: %+v", im)
	}
	if len(im.Routes) != 1 || im.Routes[0] != "rt1" || len(im.Keys) != 1 || im.Keys[0] != "k1" {
		t.Fatalf("route/key hint mismatch: %+v", im)
	}
	if err := s.cascadeDelete(sc); err != nil {
		t.Fatal(err)
	}
	if count(db, &model.Instance{}) != 1 || count(db, &model.Group{}) != 1 || count(db, &model.Account{}) != 1 || count(db, &model.AccountGroup{}) != 0 {
		t.Fatal("instance cascade left residue")
	}
	// 路由/密钥不动
	if count(db, &model.Route{}) != 2 || count(db, &model.Key{}) != 2 {
		t.Fatal("routes/keys must not be deleted")
	}
}

func TestRuleDuplicated(t *testing.T) {
	db, ids := seedCascade(t)
	s := &Server{db: db}
	// 与 r3（all 范围）完全一致 → 重复
	dup := &model.TaskRule{PluginID: ids["p"], CapabilityID: "c", TriggerType: "daily", TriggerValue: "09:00", TargetScope: "all", TargetJSON: "[]"}
	if !s.ruleDuplicated(dup, 0) {
		t.Fatal("should detect duplicate of r3")
	}
	// 排除自身则不算重复
	if s.ruleDuplicated(dup, ids["r3"]) {
		t.Fatal("excludeID must skip self")
	}
	// 触发值不同 → 不重复
	diff := &model.TaskRule{PluginID: ids["p"], CapabilityID: "c", TriggerType: "daily", TriggerValue: "10:00", TargetScope: "all", TargetJSON: "[]"}
	if s.ruleDuplicated(diff, 0) {
		t.Fatal("different trigger_value must not be duplicate")
	}
}

func TestCascadePlugin(t *testing.T) {
	db, ids := seedCascade(t)
	s := &Server{db: db}
	sc := scopePlugin(db, ids["p"])
	im := s.impact(sc)
	if im.Instances != 2 || im.Groups != 2 || im.Accounts != 3 || im.TaskRules != 3 || im.TaskRuns != 2 || len(im.Routes) != 2 {
		t.Fatalf("unexpected impact: %+v", im)
	}
	if err := s.cascadeDelete(sc); err != nil {
		t.Fatal(err)
	}
	for _, m := range []interface{}{&model.Instance{}, &model.Group{}, &model.Account{}, &model.TaskRule{}, &model.TaskRun{}} {
		if count(db, m) != 0 {
			t.Fatalf("%T not cleared", m)
		}
	}
	var rt2 model.Route
	db.First(&rt2, "name = ?", "rt2")
	if rt2.FailoverGroupID != nil {
		t.Fatal("failover_group_id should be set null by FK")
	}
}
