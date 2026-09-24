package plugin

import (
	"context"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// testStoreDB 建一个临时文件 sqlite（跨 HostService 实例可复读，验证持久化）+ plugin_stores 表。
func testStoreDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Exec(`CREATE TABLE plugin_stores (plugin TEXT NOT NULL, key TEXT NOT NULL, value BLOB, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (plugin, key))`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { // 先关连接释放文件锁，再由 TempDir 清理（Windows 必需）
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// TestStoreIsolation 验证 forPlugin 派生的宿主视图按插件名隔离 store：一个插件读不到另一个插件的同名 key。
func TestStoreIsolation(t *testing.T) {
	base := NewHostService(testStoreDB(t))
	a := base.forPlugin("plugin-a")
	b := base.forPlugin("plugin-b")
	ctx := context.Background()

	if _, err := a.StorePut(ctx, &pb.StorePutRequest{Key: "k", Value: []byte("va")}); err != nil {
		t.Fatalf("StorePut: %v", err)
	}
	if rb, _ := b.StoreGet(ctx, &pb.StoreGetRequest{Key: "k"}); rb.Found {
		t.Error("plugin-b 不应看到 plugin-a 的 key（未隔离）")
	}
	ra, _ := a.StoreGet(ctx, &pb.StoreGetRequest{Key: "k"})
	if !ra.Found || string(ra.Value) != "va" {
		t.Errorf("plugin-a 应读到自己的 key，got found=%v val=%q", ra.Found, ra.Value)
	}
}

// TestStorePersistence 验证写入落库：用新的 HostService 实例（模拟核心重启）读回，值仍在；UPSERT 覆盖生效。
func TestStorePersistence(t *testing.T) {
	db := testStoreDB(t)
	ctx := context.Background()

	h1 := NewHostService(db).forPlugin("autoclaw")
	if _, err := h1.StorePut(ctx, &pb.StorePutRequest{Key: "cursor", Value: []byte("42")}); err != nil {
		t.Fatalf("StorePut: %v", err)
	}
	if _, err := h1.StorePut(ctx, &pb.StorePutRequest{Key: "cursor", Value: []byte("99")}); err != nil {
		t.Fatalf("StorePut overwrite: %v", err)
	}
	h2 := NewHostService(db).forPlugin("autoclaw") // 模拟重启后的新实例，读同一 db
	got, _ := h2.StoreGet(ctx, &pb.StoreGetRequest{Key: "cursor"})
	if !got.Found || string(got.Value) != "99" {
		t.Errorf("持久化读回失败：found=%v val=%q，want 99", got.Found, got.Value)
	}
}
