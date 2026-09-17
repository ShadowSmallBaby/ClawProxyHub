// pluginsettings.go — 插件设置：schema 由 manifest 声明，值由管理界面在线编辑。
package admin

import (
	"encoding/json"
	"net/http"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// pluginSettings GET /admin/plugins/{name}/settings — schema + 当前值。
func (s *Server) pluginSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var rec model.Plugin
	if err := s.db.Where("name = ?", name).First(&rec).Error; err != nil {
		http.Error(w, `{"error":"plugin not found"}`, http.StatusNotFound)
		return
	}
	// schema 取运行中插件声明的（未启动插件回退 DB 快照）
	schema := rec.ManifestJSON
	if inst, ok := s.plugins.Get(name); ok {
		schema = inst.Manifest.GetSettingsSchema()
	} else {
		var m struct {
			SettingsSchema string `json:"settingsSchema"`
		}
		if json.Unmarshal([]byte(rec.ManifestJSON), &m) == nil && m.SettingsSchema != "" {
			schema = m.SettingsSchema
		}
	}
	values := rec.SettingsJSON
	if values == "" {
		values = "{}"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"schema": jsonOrNull(schema),
		"values": jsonOrNull(values),
	})
}

// putPluginSettings PUT /admin/plugins/{name}/settings — body: {values: {...}}。
// 保存后插件经 ClawHost.GetSettings 即时读到新值（无缓存）。
func (s *Server) putPluginSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var rec model.Plugin
	if err := s.db.Where("name = ?", name).First(&rec).Error; err != nil {
		http.Error(w, `{"error":"plugin not found"}`, http.StatusNotFound)
		return
	}
	var body struct {
		Values json.RawMessage `json:"values"`
	}
	if !readBody(w, r, &body) || len(body.Values) == 0 {
		http.Error(w, `{"error":"values required"}`, http.StatusBadRequest)
		return
	}
	var check map[string]interface{}
	if err := json.Unmarshal(body.Values, &check); err != nil {
		http.Error(w, `{"error":"values 必须是 JSON 对象"}`, http.StatusBadRequest)
		return
	}
	s.db.Model(&rec).Update("settings_json", string(body.Values))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
