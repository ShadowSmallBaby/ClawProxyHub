// auth.go — 管理员鉴权与首次启动引导。
// 账号存 users 表（bcrypt）；CPH_ADMIN_USERNAME/PASSWORD 仅在表空时作为引导注入。
package admin

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// ctxKeyRole 请求上下文里的当前角色键。
type ctxKeyRole struct{}

// roleOf 从上下文取角色，缺省 guest。
func roleOf(r *http.Request) string {
	if v, ok := r.Context().Value(ctxKeyRole{}).(string); ok && v != "" {
		return v
	}
	return "guest"
}

// menusForRole 角色可见菜单键：admin 全量，guest 只读（隐藏系统设置）。
func menusForRole(role string) []string {
	all := []string{"dashboard", "plugins", "accounts", "groups", "proxies", "routes", "keys", "tasks", "logs", "settings"}
	if role == "admin" {
		return all
	}
	// guest：只读概览与日志，隐藏配置类
	return []string{"dashboard", "logs"}
}

// ensureAdminSeed 表空且环境变量有密码时自动建号（容器部署引导）。
func (s *Server) ensureAdminSeed() {
	var count int64
	s.db.Model(&model.User{}).Count(&count)
	if count > 0 {
		return
	}
	password := os.Getenv("CPH_ADMIN_PASSWORD")
	if password == "" {
		return
	}
	username := os.Getenv("CPH_ADMIN_USERNAME")
	if username == "" {
		username = "admin"
	}
	s.createUser(username, password)
}

// createUser 建管理员账号。
func (s *Server) createUser(username, password string) bool {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false
	}
	return s.db.Create(&model.User{Username: username, PasswordHash: string(hash), Role: "admin"}).Error == nil
}

// auth 管理员鉴权：只认 JWT Bearer（解析 role，免 bcrypt）。
// 鉴权后把角色注入上下文；guest 只读（非 GET 请求拒绝）。
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := s.parseBearer(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		// guest 只读：仅放行 GET（写操作需 admin）
		if c.Role != "admin" && r.Method != http.MethodGet {
			http.Error(w, `{"error":"forbidden: read-only role"}`, http.StatusForbidden)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKeyRole{}, c.Role)))
	}
}

// parseBearer 取 Authorization: Bearer <jwt> 并校验，返回载荷。
func (s *Server) parseBearer(r *http.Request) (*jwtClaims, error) {
	auth := r.Header.Get("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == auth || token == "" {
		return nil, errors.New("missing bearer token")
	}
	return s.parseJWT(token)
}

// roleOfUser 查用户角色；username 空按唯一管理员。
func (s *Server) roleOfUser(username string) string {
	var user model.User
	var err error
	if username != "" {
		err = s.db.Where("username = ?", username).First(&user).Error
	} else {
		err = s.db.Where("role = ?", "admin").Order("id").First(&user).Error
	}
	if err != nil || user.Role == "" {
		return "guest"
	}
	return user.Role
}

// login POST /admin/login — 校验用户名密码，签发 JWT（免鉴权入口）。
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readBody(w, r, &body) {
		return
	}
	if !s.verifyPassword(body.Username, body.Password) {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}
	role := s.roleOfUser(body.Username)
	token := s.signJWT(body.Username, role)
	writeJSON(w, http.StatusOK, map[string]interface{}{"token": token, "role": role})
}

// verifyPassword 校验；username 为空时按唯一管理员匹配。
func (s *Server) verifyPassword(username, password string) bool {
	var user model.User
	var err error
	if username != "" {
		err = s.db.Where("username = ?", username).First(&user).Error
	} else {
		err = s.db.Where("role = ?", "admin").Order("id").First(&user).Error
	}
	if err != nil {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil
}

// lookupUsername 从 JWT 取用户名（改密场景定位记录）；缺失回退唯一管理员。
func (s *Server) lookupUsername(r *http.Request) string {
	if c, err := s.parseBearer(r); err == nil && c.Sub != "" {
		return c.Sub
	}
	var user model.User
	if err := s.db.Where("role = ?", "admin").Order("id").First(&user).Error; err == nil {
		return user.Username
	}
	return ""
}

// me GET /admin/me — 当前登录用户信息（用户名 + 角色，头像前端预留）。
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	username := s.lookupUsername(r)
	if username == "" {
		http.Error(w, `{"error":"no admin account"}`, http.StatusNotFound)
		return
	}
	var user model.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"username": user.Username,
		"role":     user.Role,
		"menus":    menusForRole(user.Role),
	})
}

// initialized users 表已有管理员账号。
func (s *Server) initialized() bool {
	var count int64
	s.db.Model(&model.User{}).Where("role = ?", "admin").Count(&count)
	return count > 0
}

// setupStatus GET /admin/setup-status — 是否需要首启引导（免鉴权）。
func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"initialized": s.initialized()})
}

// setup POST /admin/setup — 首启创建管理员账号（仅表空时可用）。
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if s.initialized() {
		http.Error(w, `{"error":"already initialized"}`, http.StatusConflict)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readBody(w, r, &body) {
		return
	}
	if err := validateCredentials(body.Username, body.Password); err != "" {
		http.Error(w, `{"error":"`+err+`"}`, http.StatusBadRequest)
		return
	}
	if !s.createUser(body.Username, body.Password) {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"initialized": true})
}

// changePassword POST /admin/password — 修改当前管理员密码（需已登录）。
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !readBody(w, r, &body) {
		return
	}
	if len(body.Password) < 6 {
		http.Error(w, `{"error":"密码至少 6 位"}`, http.StatusBadRequest)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	username := s.lookupUsername(r)
	if username == "" {
		http.Error(w, `{"error":"no admin account"}`, http.StatusNotFound)
		return
	}
	s.db.Model(&model.User{}).Where("username = ?", username).
		Update("password_hash", string(hash))
	writeJSON(w, http.StatusOK, map[string]bool{"changed": true})
}

func validateCredentials(username, password string) string {
	if username == "" || len(username) > 32 {
		return "用户名必填且不超过 32 字符"
	}
	if len(password) < 6 {
		return "密码至少 6 位"
	}
	return ""
}
