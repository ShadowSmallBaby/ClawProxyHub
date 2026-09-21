// host.go — 核心侧 ClawHost 服务：插件经 broker 反调的统一入口。
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"

	"gorm.io/gorm"

	"google.golang.org/grpc"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// HostService ClawHost gRPC 实现。状态先走内存，持久化随 storage 模块接入。
type HostService struct {
	pb.UnimplementedClawHostServer

	db     *gorm.DB
	mu     sync.RWMutex
	stores map[string]map[string][]byte // plugin name → key → value
}

func NewHostService(db *gorm.DB) *HostService {
	return &HostService{db: db, stores: map[string]map[string][]byte{}}
}

func (h *HostService) Log(ctx context.Context, e *pb.LogEntry) (*pb.Empty, error) {
	log.Printf("[plugin] %s: %s", e.Level, e.Message)
	return &pb.Empty{}, nil
}

func (h *HostService) StoreGet(ctx context.Context, r *pb.StoreGetRequest) (*pb.StoreGetResponse, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	// TODO: 按插件隔离 key 空间（grpc peer → plugin name）
	v, ok := h.stores[""][r.Key]
	return &pb.StoreGetResponse{Value: v, Found: ok}, nil
}

func (h *HostService) StorePut(ctx context.Context, r *pb.StorePutRequest) (*pb.Empty, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stores[""] == nil {
		h.stores[""] = map[string][]byte{}
	}
	h.stores[""][r.Key] = r.Value
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
