// proxy.go — 分组出站代理解析：账号 → 分组 → group_proxies → 代理配置。
package account

import (
	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// ProxyForAccount 账号任一分组绑定的代理（任务调度等无分组上下文的场景）。
func ProxyForAccount(db *gorm.DB, accountID int64) *pb.ProxyConfig {
	var ag model.AccountGroup
	if err := db.Where("account_id = ?", accountID).Order("group_id").First(&ag).Error; err != nil {
		return nil
	}
	return ProxyForGroup(db, ag.GroupID)
}

// ProxyForAccountIn 账号在命中分组下的代理（多分组账号优先走路由命中的分组出站）。
func ProxyForAccountIn(db *gorm.DB, accountID, groupID int64) *pb.ProxyConfig {
	if px := ProxyForGroup(db, groupID); px != nil {
		return px
	}
	return ProxyForAccount(db, accountID)
}

// ProxyForGroup 分组绑定的首个代理（Host.GetProxy 回调用）。
func ProxyForGroup(db *gorm.DB, groupID int64) *pb.ProxyConfig {
	var links []model.GroupProxy
	if err := db.Where("group_id = ?", groupID).Order("proxy_id").Find(&links).Error; err != nil || len(links) == 0 {
		return nil
	}
	var proxy model.Proxy
	if err := db.First(&proxy, links[0].ProxyID).Error; err != nil {
		return nil
	}
	return &pb.ProxyConfig{
		Scheme: proxy.Scheme, Host: proxy.Host, Port: proxy.Port,
		Username: proxy.Username, Password: proxy.Password,
	}
}
