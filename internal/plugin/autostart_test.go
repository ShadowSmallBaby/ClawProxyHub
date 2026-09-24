package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/database"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// AutoStarts 排除持久化停止（enabled=0）的插件；无记录 / 未停用的照常拉起。
func TestAutoStartsSkipsDisabled(t *testing.T) {
	root := t.TempDir()
	mk := func(name string) {
		dir := filepath.Join(root, name)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"name":"`+name+`"}`), 0o644)
		bin := filepath.Join(dir, "plugin-"+runtimeOS()+"-"+runtimeArch())
		if runtimeOS() == "windows" {
			bin += ".exe"
		}
		os.WriteFile(bin, []byte("bin"), 0o755)
	}
	mk("alpha") // 有记录且 enabled=1
	mk("beta")  // 持久化停止 enabled=0
	mk("gamma") // 无 DB 记录（新装）

	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	db.Create(&model.Plugin{Name: "alpha", Version: "1", Author: "a", ManifestJSON: "{}", Enabled: true})
	// Enabled 带 default:true，Create 传零值 false 会被 gorm 当"未设置"→ 用 Update 显式落 0（同生产 Stop 路径）
	db.Create(&model.Plugin{Name: "beta", Version: "1", Author: "a", ManifestJSON: "{}"})
	db.Model(&model.Plugin{}).Where("name = ?", "beta").Update("enabled", false)

	m := &Manager{dir: root, db: db}
	dirs, err := m.AutoStarts()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, d := range dirs {
		got[filepath.Base(d)] = true // AutoStarts 现返回插件目录
	}
	if !got["alpha"] || got["beta"] || !got["gamma"] {
		t.Fatalf("autostart set wrong: %v (want alpha+gamma, not beta)", got)
	}
}
