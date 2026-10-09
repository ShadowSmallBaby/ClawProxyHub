package admin

import (
	"errors"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const deviceSetupKey = "auth.device_setup_completed"

// WithDeviceSetup 只用于本机平台宿主，兼容旧版随机密码账号的首次认领。
func WithDeviceSetup() Option { return func(s *Server) { s.deviceSetup = true } }

func (s *Server) legacyDeviceOwner(db *gorm.DB) bool {
	if !s.deviceSetup {
		return false
	}
	var marker model.Setting
	if err := db.Where("key = ?", deviceSetupKey).First(&marker).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		return false
	}
	var users []model.User
	return db.Find(&users).Error == nil && len(users) == 1 && users[0].Username == "device-owner" && users[0].Role == "admin" && users[0].AuthVersion == 0
}

// createDeviceUser 保留旧账号 ID 和业务数据，并撤销随机密码时代的会话。
func (s *Server) createDeviceUser(username, password string) bool {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if s.legacyDeviceOwner(tx) {
			if err := tx.Model(&model.User{}).Where("username = ?", "device-owner").Updates(map[string]any{"username": username, "password_hash": string(hash), "auth_version": gorm.Expr("auth_version + 1")}).Error; err != nil {
				return err
			}
		} else {
			result := tx.Exec(`INSERT INTO users (username, password_hash, role, created_at, updated_at) SELECT ?, ?, 'admin', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP WHERE NOT EXISTS (SELECT 1 FROM users WHERE role = 'admin')`, username, string(hash))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("already initialized")
			}
		}
		return tx.Exec(`INSERT INTO settings (key, value, updated_at) VALUES (?, 'true', CURRENT_TIMESTAMP) ON CONFLICT(key) DO UPDATE SET value = 'true'`, deviceSetupKey).Error
	})
	return err == nil
}

// DeviceSession 仅供原生管理功能；未完成 setup 时不建号、不签发权限。
func (s *Server) DeviceSession() (string, error) {
	if !s.initialized() {
		return "", nil
	}
	return s.signJWT("", "admin")
}
