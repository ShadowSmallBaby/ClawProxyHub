package app

import (
	accountpkg "github.com/ShadowSmallBaby/ClawProxyHub/internal/account"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"gorm.io/gorm"
)

// seedAPIKey 首次部署引导：环境变量指定 key，不存在则入库（加密存储）。
func seedAPIKey(db *gorm.DB, raw string, dataDir string) error {
	lookup := accountpkg.KeyLookupHash(raw)
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.Key{}).Where("key_lookup = ? OR key_cipher = ?", lookup, lookup).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		var old []model.Key
		if err := tx.Where("key_lookup IS NULL OR key_lookup = ''").Find(&old).Error; err != nil {
			return err
		}
		for _, key := range old {
			plain, err := accountpkg.DecryptCredential(dataDir, []byte(key.KeyCipher))
			if err != nil {
				return err
			}
			if string(plain) == raw {
				return tx.Model(&key).Update("key_lookup", lookup).Error
			}
		}
		sealed, err := accountpkg.EncryptCredential(dataDir, []byte(raw))
		if err != nil {
			return err
		}
		return tx.Create(&model.Key{KeyCipher: string(sealed), KeyLookup: lookup, Name: "seed"}).Error
	})
}
