// cred.go — 凭据信封构造（网关/刷新/任务/测试四条链路统一入口）+ 实例归属。
package account

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// BuildCred 账号 → 凭据信封：解密 blob + 刷新时间 + 实例 + 出站代理。
// groupID>0 为路由命中的分组（代理优先级：账号 > 该分组）；0 = 仅按账号绑定回退全部分组。
func BuildCred(db *gorm.DB, dataDir string, acct *model.Account, groupID int64) *pb.CredentialBlob {
	cred := &pb.CredentialBlob{
		AccountId:  fmt.Sprintf("%d", acct.ID),
		Blob:       DecryptCredential(dataDir, acct.CredentialBlob),
		InstanceId: acct.InstanceID,
	}
	if acct.LastRefreshAt != nil {
		cred.UpdatedAt = acct.LastRefreshAt.Unix()
	}
	if groupID > 0 {
		cred.Proxy = ProxyForAccountIn(db, acct.ID, groupID)
	} else {
		cred.Proxy = ProxyForAccount(db, acct.ID)
	}
	return cred
}

// DefaultInstance 单例插件的默认实例（最早创建的一个）；没有则建一个「默认」。
func DefaultInstance(db *gorm.DB, pluginID int64) (*model.Instance, error) {
	var inst model.Instance
	err := db.Where("plugin_id = ?", pluginID).Order("id").First(&inst).Error
	if err == nil {
		return &inst, nil
	}
	inst = model.Instance{PluginID: pluginID, Name: "默认", SettingsJSON: "{}"}
	if err := db.Create(&inst).Error; err != nil {
		return nil, err
	}
	return &inst, nil
}

// ResolveInstance 校验实例归属：instanceID>0 须属于该插件；
// 0 时单例插件（multi=false）落默认实例，多实例插件必须显式指定（不自动建）。
func ResolveInstance(db *gorm.DB, pluginID, instanceID int64, multi bool) (*model.Instance, error) {
	if instanceID <= 0 {
		if multi {
			return nil, fmt.Errorf("该插件支持多实例，请先创建实例并指定")
		}
		return DefaultInstance(db, pluginID)
	}
	var inst model.Instance
	if err := db.First(&inst, instanceID).Error; err != nil || inst.PluginID != pluginID {
		return nil, fmt.Errorf("实例不存在或与插件不一致")
	}
	return &inst, nil
}
