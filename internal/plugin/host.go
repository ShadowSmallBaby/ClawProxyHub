// host.go — 核心侧 ClawHost 服务：插件经 broker 反调的统一入口。
package plugin

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"

	"gorm.io/gorm"

	"google.golang.org/grpc"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
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

func (h *HostService) GetProxy(ctx context.Context, r *pb.GetProxyRequest) (*pb.ProxyConfig, error) {
	groupID, err := strconv.ParseInt(r.GroupId, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid group id")
	}
	var links []model.GroupProxy
	if err := h.db.Where("group_id = ?", groupID).Order("proxy_id").Find(&links).Error; err != nil || len(links) == 0 {
		return nil, fmt.Errorf("no proxy bound to group %s", r.GroupId)
	}
	var proxy model.Proxy
	if err := h.db.First(&proxy, links[0].ProxyID).Error; err != nil {
		return nil, fmt.Errorf("proxy record missing")
	}
	return &pb.ProxyConfig{
		Scheme: proxy.Scheme, Host: proxy.Host, Port: proxy.Port,
		Username: proxy.Username, Password: proxy.Password,
	}, nil
}

// GetSettings 读插件设置（管理界面在线修改，保存即生效）。
func (h *HostService) GetSettings(ctx context.Context, r *pb.GetSettingsRequest) (*pb.GetSettingsResponse, error) {
	var p model.Plugin
	if err := h.db.Select("settings_json").Where("name = ?", r.Plugin).First(&p).Error; err != nil {
		return &pb.GetSettingsResponse{Values: []byte("{}")}, nil // 记录缺失按空配置处理
	}
	if p.SettingsJSON == "" {
		p.SettingsJSON = "{}"
	}
	return &pb.GetSettingsResponse{Values: []byte(p.SettingsJSON)}, nil
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
