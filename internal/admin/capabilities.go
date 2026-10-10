package admin

import (
	"net/http"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/module"
)

type Option func(*Server)

// WithModules 由应用装配器提供发行组合与运行状态，不接受客户端切换能力。
func WithModules(profile string, states func() []module.State) Option {
	return func(s *Server) { s.profile, s.moduleStates = profile, states }
}

func (s *Server) capabilities(w http.ResponseWriter, r *http.Request) {
	states := []module.State{}
	if s.moduleStates != nil {
		states = s.moduleStates()
	}
	profile := s.profile
	if profile == "" {
		profile = "full"
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": profile, "modules": states})
}

func (s *Server) visibleMenus(role string) []string {
	return menusForRole(role)
}
