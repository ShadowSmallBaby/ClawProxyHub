package plugin

import (
	"context"
	"encoding/json"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// 宿主 GetSettings 实例视图：插件设置 ← 实例设置 ← base_url。
func TestHostGetSettingsInstanceView(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// 仅测试用建表（生产 schema 只经迁移 SQL）
	if err := db.AutoMigrate(&model.Plugin{}, &model.Instance{}); err != nil {
		t.Fatal(err)
	}
	db.Create(&model.Plugin{Name: "newapi", SettingsJSON: `{"user_agent":"ua","quota_per_unit":1}`})
	inst := model.Instance{PluginID: 1, Name: "主站", BaseURL: "https://api.example.com", SettingsJSON: `{"quota_per_unit":500000}`}
	db.Create(&inst)

	h := NewHostService(db)
	resp, err := h.GetSettings(context.Background(), &pb.GetSettingsRequest{Plugin: "newapi", InstanceId: inst.ID})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	json.Unmarshal(resp.Values, &got)
	if got["base_url"] != "https://api.example.com" || got["user_agent"] != "ua" || got["quota_per_unit"] != float64(500000) {
		t.Fatalf("merged view wrong: %v", got)
	}
	// 无实例：仅插件级，不含 base_url
	resp, _ = h.GetSettings(context.Background(), &pb.GetSettingsRequest{Plugin: "newapi"})
	got = map[string]interface{}{}
	json.Unmarshal(resp.Values, &got)
	if _, ok := got["base_url"]; ok || got["quota_per_unit"] != float64(1) {
		t.Fatalf("plugin-only view wrong: %v", got)
	}
}
