package account

import (
	"errors"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"gorm.io/gorm"
)

// ProxyConfig 将持久化代理转换为仅供出站使用的明文配置。
func ProxyConfig(dataDir string, px *model.Proxy) (*pb.ProxyConfig, error) {
	password := px.Password
	if len(px.PasswordCipher) > 0 {
		raw, err := DecryptCredential(dataDir, px.PasswordCipher)
		if err != nil {
			return nil, err
		}
		password = string(raw)
	}
	return &pb.ProxyConfig{Scheme: px.Scheme, Host: px.Host, Port: px.Port, Username: px.Username, Password: password}, nil
}

func ProxyForAccountIn(db *gorm.DB, dataDir string, accountID, groupID int64) (*pb.ProxyConfig, error) {
	var link model.AccountProxy
	err := db.Where("account_id = ?", accountID).Order("proxy_id").First(&link).Error
	if err == nil {
		return proxyByID(db, dataDir, link.ProxyID)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if groupID > 0 {
		px, err := ProxyForGroup(db, dataDir, groupID)
		if px != nil || err != nil {
			return px, err
		}
	}
	var ag model.AccountGroup
	err = db.Where("account_id = ?", accountID).Order("group_id").First(&ag).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ProxyForGroup(db, dataDir, ag.GroupID)
}

func ProxyForGroup(db *gorm.DB, dataDir string, groupID int64) (*pb.ProxyConfig, error) {
	var link model.GroupProxy
	err := db.Where("group_id = ?", groupID).Order("proxy_id").First(&link).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return proxyByID(db, dataDir, link.ProxyID)
}

func proxyByID(db *gorm.DB, dataDir string, id int64) (*pb.ProxyConfig, error) {
	var px model.Proxy
	if err := db.First(&px, id).Error; err != nil {
		return nil, err
	}
	return ProxyConfig(dataDir, &px)
}

// EncryptProxyPasswords 成功加密后才清除旧明文；重复执行无副作用。
func EncryptProxyPasswords(db *gorm.DB, dataDir string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var list []model.Proxy
		if err := tx.Where("password <> ''").Find(&list).Error; err != nil {
			return err
		}
		for _, px := range list {
			blob, err := EncryptCredential(dataDir, []byte(px.Password))
			if err != nil {
				return err
			}
			if err := tx.Model(&px).Updates(map[string]interface{}{"password_cipher": blob, "password": ""}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
