package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/action"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"gorm.io/gorm"
)

type State struct {
	Enabled        bool     `json:"enabled"`
	HasKey         bool     `json:"has_key"`
	AllowedActions []string `json:"allowed_actions"`
	UpdatedAt      string   `json:"updated_at"`
}
type credential struct {
	Username       string
	TokenHash      *string
	AuthVersion    int64
	AllowedActions string
	UpdatedAt      string
}
type policyKey struct{}

// Managed 与核心同进程，密钥及授权存数据库；变更与请求注册串行，撤销立即取消现有请求。
type Managed struct {
	db       *gorm.DB
	registry *action.Registry
	server   *Server
	mu       sync.Mutex
	next     uint64
	active   map[uint64]context.CancelFunc
}

func NewManager(db *gorm.DB, registry *action.Registry) *Managed {
	m := &Managed{db: db, registry: registry, active: map[uint64]context.CancelFunc{}}
	m.server = NewManaged(registry, func(ctx context.Context, _ action.Principal, id string) bool {
		allowed, _ := ctx.Value(policyKey{}).([]string)
		return slices.Contains(allowed, id)
	})
	return m
}
func (m *Managed) user(ctx context.Context, subject string) (model.User, error) {
	var user model.User
	err := m.db.WithContext(ctx).Where("username = ? AND role = ?", subject, "admin").First(&user).Error
	return user, err
}
func (m *Managed) State(ctx context.Context, subject string) (State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state(ctx, subject)
}
func (m *Managed) state(ctx context.Context, subject string) (State, error) {
	state := State{AllowedActions: []string{}}
	user, err := m.user(ctx, subject)
	if err != nil {
		return state, err
	}
	var settings struct{ Enabled bool }
	if err = m.db.WithContext(ctx).Table("mcp_settings").Where("id = 1").Take(&settings).Error; err != nil {
		return state, err
	}
	state.Enabled = settings.Enabled
	var key credential
	err = m.db.WithContext(ctx).Table("mcp_credentials").Where("username = ?", subject).Take(&key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	state.HasKey = key.TokenHash != nil && key.AuthVersion == user.AuthVersion
	state.UpdatedAt = key.UpdatedAt
	err = json.Unmarshal([]byte(key.AllowedActions), &state.AllowedActions)
	return state, err
}
func (m *Managed) cancelActive() {
	for _, cancel := range m.active {
		cancel()
	}
}
func (m *Managed) Configure(ctx context.Context, subject string, enabled bool, allowed []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.user(ctx, subject); err != nil {
		return err
	}
	known := map[string]bool{}
	for _, tool := range m.registry.List(action.Principal{Subject: subject, Role: "admin"}) {
		known[tool.ID] = true
	}
	if len(allowed) > 512 {
		return errors.New("too many tools")
	}
	selected := map[string]bool{}
	for _, id := range allowed {
		if !known[id] || selected[id] {
			return errors.New("unknown or duplicate tool")
		}
		selected[id] = true
	}
	if allowed == nil {
		allowed = []string{}
	}
	raw, _ := json.Marshal(allowed)
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE mcp_settings SET enabled = ? WHERE id = 1", enabled).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO mcp_credentials(username,allowed_actions) VALUES (?,?) ON CONFLICT(username) DO UPDATE SET allowed_actions=excluded.allowed_actions,updated_at=CURRENT_TIMESTAMP`, subject, string(raw)).Error
	})
	if err == nil {
		m.cancelActive()
	}
	return err
}
func (m *Managed) Rotate(ctx context.Context, subject string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	user, err := m.user(ctx, subject)
	if err != nil {
		return "", err
	}
	bytes := make([]byte, 32)
	if _, err = rand.Read(bytes); err != nil {
		return "", err
	}
	token := "cph_mcp_" + hex.EncodeToString(bytes)
	hash := sha256.Sum256([]byte(token))
	digest := hex.EncodeToString(hash[:])
	err = m.db.WithContext(ctx).Exec(`INSERT INTO mcp_credentials(username,token_hash,auth_version) VALUES (?,?,?) ON CONFLICT(username) DO UPDATE SET token_hash=excluded.token_hash,auth_version=excluded.auth_version,updated_at=CURRENT_TIMESTAMP`, subject, digest, user.AuthVersion).Error
	if err != nil {
		return "", err
	}
	m.cancelActive()
	return token, nil
}
func (m *Managed) Revoke(ctx context.Context, subject string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.user(ctx, subject); err != nil {
		return err
	}
	err := m.db.WithContext(ctx).Exec("UPDATE mcp_credentials SET token_hash=NULL,updated_at=CURRENT_TIMESTAMP WHERE username=?", subject).Error
	if err == nil {
		m.cancelActive()
	}
	return err
}
func (m *Managed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if origin := r.Header.Get("Origin"); origin != "" && origin != "https://"+r.Host && origin != "http://"+r.Host {
		http.Error(w, "MCP cross-origin request rejected", 403)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == r.Header.Get("Authorization") || !strings.HasPrefix(token, "cph_mcp_") || len(token) != 72 {
		http.Error(w, "invalid MCP credential", 401)
		return
	}
	digest := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(digest[:])
	m.mu.Lock()
	var key credential
	err := m.db.WithContext(r.Context()).Table("mcp_credentials").Where("token_hash = ?", hash).Take(&key).Error
	var state State
	if err == nil {
		state, err = m.state(r.Context(), key.Username)
	}
	if err != nil || !state.HasKey {
		m.mu.Unlock()
		http.Error(w, "MCP credential revoked or unavailable", 401)
		return
	}
	if !state.Enabled {
		m.mu.Unlock()
		http.Error(w, "MCP service is disabled", 503)
		return
	}
	ctx, cancel := context.WithCancel(context.WithValue(r.Context(), policyKey{}, state.AllowedActions))
	m.next++
	id := m.next
	m.active[id] = cancel
	m.mu.Unlock()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				user, err := m.user(ctx, key.Username)
				if err != nil || user.AuthVersion != key.AuthVersion {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); m.mu.Lock(); delete(m.active, id); m.mu.Unlock() }()
	m.server.HTTP(w, r.WithContext(ctx), action.Principal{Subject: key.Username, Role: "admin"}, hash)
}
