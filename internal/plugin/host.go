// host.go — 核心侧 ClawHost 服务：插件经 broker 反调的统一入口。
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"google.golang.org/grpc"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/runlog"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// HostService ClawHost gRPC 实现。每个插件实例经 forPlugin 派生独立视图，store 按插件名隔离并持久化到 DB。
type HostService struct {
	pb.UnimplementedClawHostServer

	db     *gorm.DB
	plugin string // 绑定的插件名（store 命名空间）；空 = 未绑定的模板
}

func NewHostService(db *gorm.DB) *HostService {
	return &HostService{db: db}
}

// forPlugin 派生一个绑定到具体插件的宿主视图（共享 db），store 按 plugin 名隔离。
func (h *HostService) forPlugin(name string) *HostService {
	return &HostService{db: h.db, plugin: name}
}

// runLogger 运行日志写入器（级别设置实时读库）。
func (h *HostService) runLogger() *runlog.Logger {
	return runlog.New(h.db, func() string { return setting.New(h.db).RunLevel() })
}

// runActions 固定 action 词表（语义化；插件自定义的 action 未命中按 "other"）。
var runActions = map[string]bool{
	"chat": true, "login": true, "refresh": true, "profile": true,
	"models": true, "http": true, "task": true, "store": true,
}

func (h *HostService) Log(ctx context.Context, e *pb.LogEntry) (*pb.Empty, error) {
	log.Printf("[plugin] %s: %s", e.Level, e.Message)
	// Fields 约定键：action（语义化操作，未命中词表按 other）+ detail（排查明细）
	action, detail := "other", ""
	if e.Fields != nil {
		if a := e.Fields["action"]; a != "" {
			if !runActions[a] {
				a = "other"
			}
			action = a
		}
		detail = e.Fields["detail"]
	}
	h.runLogger().Log(e.Level, "plugin", action, e.Message, detail, nil)
	return &pb.Empty{}, nil
}

func (h *HostService) StoreGet(ctx context.Context, r *pb.StoreGetRequest) (*pb.StoreGetResponse, error) {
	// 用 Find（而非 First）避免命中不到时 GORM 记 record-not-found 日志噪声。
	var recs []model.PluginStore
	if err := h.db.Where("plugin = ? AND key = ?", h.plugin, r.Key).Limit(1).Find(&recs).Error; err != nil {
		h.runLogger().Warn("plugin", "store", "store get 失败: "+r.Key, err.Error(), nil)
		return &pb.StoreGetResponse{Found: false}, nil
	}
	if len(recs) == 0 {
		return &pb.StoreGetResponse{Found: false}, nil
	}
	return &pb.StoreGetResponse{Value: recs[0].Value, Found: true}, nil
}

func (h *HostService) StorePut(ctx context.Context, r *pb.StorePutRequest) (*pb.Empty, error) {
	rec := model.PluginStore{Plugin: h.plugin, Key: r.Key, Value: r.Value, UpdatedAt: time.Now()}
	// UPSERT：(plugin,key) 冲突则更新 value/updated_at
	if err := h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "plugin"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&rec).Error; err != nil {
		h.runLogger().Warn("plugin", "store", "store put 失败: "+r.Key, err.Error(), nil)
	}
	return &pb.Empty{}, nil
}

// GetProxy 取出站代理：account_id 非空时账号级绑定优先，再回退 group_id 的分组绑定。
func (h *HostService) GetProxy(ctx context.Context, r *pb.GetProxyRequest) (*pb.ProxyConfig, error) {
	if r.AccountId != "" {
		if accountID, err := strconv.ParseInt(r.AccountId, 10, 64); err == nil {
			var link model.AccountProxy
			if err := h.db.Where("account_id = ?", accountID).Order("proxy_id").First(&link).Error; err == nil {
				if p, err := h.proxyByID(link.ProxyID); err == nil {
					return p, nil
				}
			}
		}
	}
	groupID, err := strconv.ParseInt(r.GroupId, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid group id")
	}
	var links []model.GroupProxy
	if err := h.db.Where("group_id = ?", groupID).Order("proxy_id").Find(&links).Error; err != nil || len(links) == 0 {
		return nil, fmt.Errorf("no proxy bound to group %s", r.GroupId)
	}
	return h.proxyByID(links[0].ProxyID)
}

func (h *HostService) proxyByID(id int64) (*pb.ProxyConfig, error) {
	var proxy model.Proxy
	if err := h.db.First(&proxy, id).Error; err != nil {
		return nil, fmt.Errorf("proxy record missing")
	}
	return &pb.ProxyConfig{
		Scheme: proxy.Scheme, Host: proxy.Host, Port: proxy.Port,
		Username: proxy.Username, Password: proxy.Password,
	}, nil
}

// GetSettings 读插件设置（管理界面在线修改，保存即生效）。
// instance_id>0 时返回合并视图：插件设置 ← 实例设置 ← {"base_url": 实例地址, "instance_name": 实例名}（后者覆盖前者）。
// 另注入保留键 sdk.SettingBrowserUserAgent（全局浏览器 UA，非空才给）。
func (h *HostService) GetSettings(ctx context.Context, r *pb.GetSettingsRequest) (*pb.GetSettingsResponse, error) {
	merged := map[string]json.RawMessage{}
	var p model.Plugin
	if err := h.db.Select("settings_json").Where("name = ?", r.Plugin).First(&p).Error; err == nil {
		mergeJSON(merged, p.SettingsJSON)
	}
	if r.InstanceId > 0 {
		var inst model.Instance
		if err := h.db.First(&inst, r.InstanceId).Error; err == nil {
			mergeJSON(merged, inst.SettingsJSON)
			if inst.BaseURL != "" {
				merged["base_url"], _ = json.Marshal(inst.BaseURL)
			}
			merged["instance_name"], _ = json.Marshal(inst.Name)
		}
	}
	if ua := setting.New(h.db).BrowserUserAgent(); ua != "" {
		merged[sdk.SettingBrowserUserAgent], _ = json.Marshal(ua)
	}
	out, err := json.Marshal(merged)
	if err != nil {
		out = []byte("{}")
	}
	return &pb.GetSettingsResponse{Values: out}, nil
}

// mergeJSON 把 JSON 对象的顶层键并入 dst（非对象/解析失败忽略）。
func mergeJSON(dst map[string]json.RawMessage, src string) {
	if src == "" {
		return
	}
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(src), &m) == nil {
		for k, v := range m {
			dst[k] = v
		}
	}
}

// ServeHost 在 broker 上挂出宿主服务（由 ClawPluginPlugin.GRPCClient 调用）。
func (h *HostService) ServeHost(broker interface {
	AcceptAndServe(id uint32, f func([]grpc.ServerOption) *grpc.Server)
}) {
	broker.AcceptAndServe(hostBrokerID, func(opts []grpc.ServerOption) *grpc.Server {
		srv := grpc.NewServer(opts...)
		pb.RegisterClawHostServer(srv, h)
		return srv
	})
}
