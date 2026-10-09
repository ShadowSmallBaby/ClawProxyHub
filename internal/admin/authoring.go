// authoring.go — 用户自建 Lua 插件的在线编辑 API。
package admin

import (
	"io"
	"net/http"
)

// pluginScaffold GET /admin/plugins/scaffold — 内置脚手架 main.lua。
func (s *Server) pluginScaffold(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"lua": s.plugins.ScaffoldLua("")})
}

// createLocalPlugin POST /admin/plugins/local — multipart: {name, label, lua, icon?}
// 新建自建插件（脚手架 manifest + 用户 main.lua + 可选 icon 落盘，不执行）。
func (s *Server) createLocalPlugin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, `{"error":"invalid upload"}`, http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()
	name := r.FormValue("name")
	label := r.FormValue("label")
	lua := r.FormValue("lua")
	if name == "" || lua == "" {
		http.Error(w, `{"error":"name and lua required"}`, http.StatusBadRequest)
		return
	}
	var icon []byte
	if fh := r.MultipartForm.File["icon"]; len(fh) > 0 {
		f, err := fh[0].Open()
		if err != nil {
			http.Error(w, `{"error":"read icon"}`, http.StatusBadRequest)
			return
		}
		defer f.Close()
		icon, err = io.ReadAll(io.LimitReader(f, 2<<20)) // 2MB 上限
		if err != nil {
			http.Error(w, `{"error":"read icon"}`, http.StatusBadRequest)
			return
		}
	}
	created, err := s.plugins.CreateLocalWorkspace(name, label, lua, icon)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.db.Exec(`INSERT OR IGNORE INTO plugins (name, version, author, protocol_version, manifest_json, source) VALUES (?,?,?,?,?,?)`,
		created, "", "local", 0, "{}", "local")
	s.db.Exec(`UPDATE plugins SET source = ? WHERE name = ?`, "local", created)
	s.plugins.RefreshCatalog(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"created": created})
}

// getPluginSource GET /admin/plugins/{name}/source?file= — 读自建插件源码。
func (s *Server) getPluginSource(w http.ResponseWriter, r *http.Request) {
	file := r.URL.Query().Get("file")
	if file == "" {
		file = "main.lua"
	}
	content, err := s.plugins.ReadLocalSource(r.PathValue("name"), file)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": content})
}

// putPluginSource PUT /admin/plugins/{name}/source — body: {file, content} 仅保存，执行通过独立动作授权。
func (s *Server) putPluginSource(w http.ResponseWriter, r *http.Request) {
	var body struct {
		File    string `json:"file"`
		Content string `json:"content"`
	}
	if !readBody(w, r, &body) || body.File == "" {
		http.Error(w, `{"error":"file required"}`, http.StatusBadRequest)
		return
	}
	if err := s.plugins.SaveLocalSource(r.PathValue("name"), body.File, body.Content); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.plugins.RefreshCatalog(r.Context())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
