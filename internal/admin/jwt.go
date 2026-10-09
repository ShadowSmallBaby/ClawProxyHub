// jwt.go — 管理会话签名与按用户版本撤销。
package admin

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"gorm.io/gorm"
)

const keyJWTSecret = "auth.jwt_secret"
const jwtTTL = 7 * 24 * time.Hour

type jwtClaims struct {
	Scoped  bool     `json:"scoped,omitempty"`
	Scopes  []string `json:"scopes,omitempty"`
	Nonce   string   `json:"nonce,omitempty"`
	Sub     string   `json:"sub"`
	Role    string   `json:"role"`
	Iat     int64    `json:"iat"`
	Exp     int64    `json:"exp"`
	Version *int64   `json:"ver"`
}

func (s *Server) jwtSecret() ([]byte, error) {
	s.jwtMu.Lock()
	defer s.jwtMu.Unlock()
	if len(s.jwtKey) > 0 {
		return s.jwtKey, nil
	}
	var rec model.Setting
	err := s.db.Where("key = ?", keyJWTSecret).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		buf := make([]byte, 32)
		rand.Read(buf)
		if err := s.db.Exec(`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP) ON CONFLICT(key) DO NOTHING`, keyJWTSecret, hex.EncodeToString(buf)).Error; err != nil {
			return nil, err
		}
		err = s.db.Where("key = ?", keyJWTSecret).First(&rec).Error
	}
	if err != nil {
		return nil, err
	}
	if len(rec.Value) < 32 {
		return nil, errors.New("invalid JWT secret")
	}
	s.jwtKey = []byte(rec.Value)
	return s.jwtKey, nil
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (s *Server) signJWT(username, role string) (string, error) {
	return s.signSession(username, nil, jwtTTL)
}

func (s *Server) signSession(username string, scopes []string, ttl time.Duration) (string, error) {
	var user model.User
	q := s.db.Where("username = ?", username)
	if username == "" {
		q = s.db.Where("role = ?", "admin").Order("id")
	}
	if err := q.First(&user).Error; err != nil {
		return "", err
	}
	key, err := s.jwtSecret()
	if err != nil {
		return "", err
	}
	now := time.Now()
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	payload, err := json.Marshal(jwtClaims{Sub: user.Username, Role: user.Role, Iat: now.Unix(), Exp: now.Add(ttl).Unix(), Version: &user.AuthVersion, Scoped: scopes != nil, Scopes: scopes, Nonce: hex.EncodeToString(nonce)})
	if err != nil {
		return "", err
	}
	signing := b64url([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + b64url(payload)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(signing))
	return signing + "." + b64url(mac.Sum(nil)), nil
}

func (s *Server) parseJWT(token string) (*jwtClaims, error) {
	if s.legacyDeviceOwner(s.db) {
		return nil, errors.New("device setup required")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	key, err := s.jwtSecret()
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal([]byte(b64url(mac.Sum(nil))), []byte(parts[2])) {
		return nil, errors.New("bad signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var c jwtClaims
	if json.Unmarshal(raw, &c) != nil || c.Sub == "" || c.Version == nil || time.Now().Unix() >= c.Exp {
		return nil, errors.New("invalid or expired claims")
	}
	var user model.User
	if err := s.db.Where("username = ?", c.Sub).First(&user).Error; err != nil {
		return nil, err
	}
	if *c.Version != user.AuthVersion || (user.Role != "admin" && user.Role != "guest") {
		return nil, errors.New("session revoked")
	}
	c.Role = user.Role
	return &c, nil
}
