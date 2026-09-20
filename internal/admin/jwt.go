// jwt.go — 管理会话 JWT（HS256，标准库实现，免第三方依赖）。
// 签名密钥存 settings（auth.jwt_secret），首次缺失时生成随机值落库。
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
)

// keyJWTSecret HS256 签名密钥的 settings key。
const keyJWTSecret = "auth.jwt_secret"

// jwtTTL 令牌有效期。
const jwtTTL = 7 * 24 * time.Hour

// jwtClaims 令牌载荷：管理会话必要字段。
type jwtClaims struct {
	Sub  string `json:"sub"`  // 用户名
	Role string `json:"role"` // admin / guest
	Iat  int64  `json:"iat"`  // 签发时间（Unix 秒）
	Exp  int64  `json:"exp"`  // 过期时间（Unix 秒）
}

// jwtSecret 取签名密钥，缺失时生成随机 32 字节并落库。
func (s *Server) jwtSecret() []byte {
	sec := s.settings.Get(keyJWTSecret, "")
	if sec == "" {
		buf := make([]byte, 32)
		rand.Read(buf)
		sec = hex.EncodeToString(buf)
		s.settings.Set(keyJWTSecret, sec)
	}
	return []byte(sec)
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// signJWT 签发 HS256 令牌。
func (s *Server) signJWT(username, role string) string {
	header := b64url([]byte(`{"alg":"HS256","typ":"JWT"}`))
	now := time.Now()
	payload, _ := json.Marshal(jwtClaims{
		Sub: username, Role: role,
		Iat: now.Unix(), Exp: now.Add(jwtTTL).Unix(),
	})
	signing := header + "." + b64url(payload)
	mac := hmac.New(sha256.New, s.jwtSecret())
	mac.Write([]byte(signing))
	return signing + "." + b64url(mac.Sum(nil))
}

// parseJWT 校验签名与过期，返回载荷。
func (s *Server) parseJWT(token string) (*jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	signing := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, s.jwtSecret())
	mac.Write([]byte(signing))
	if !hmac.Equal([]byte(b64url(mac.Sum(nil))), []byte(parts[2])) {
		return nil, errors.New("bad signature")
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("bad payload")
	}
	var c jwtClaims
	if err := json.Unmarshal(payloadRaw, &c); err != nil {
		return nil, errors.New("bad claims")
	}
	if time.Now().Unix() > c.Exp {
		return nil, errors.New("token expired")
	}
	return &c, nil
}
