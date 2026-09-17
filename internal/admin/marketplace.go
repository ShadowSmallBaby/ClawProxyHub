// marketplace.go — 插件市场与包安装。
package admin

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/plugin"
)

// 离线插件市场索引（线上索引不可达时的兜底清单）
// 内容是 ClawProxyHubPlugins 仓库 index.json 的快照，随核心发布同步；sha256 为空表示跳过校验。
//
//go:embed offline_market.json
var offlineMarketJSON []byte

// marketHTTPClient 市场请求统一走带超时的 client，避免网络不通时挂死。
var marketHTTPClient = &http.Client{Timeout: 20 * time.Second}

// offlineMarket 解析内置离线索引。
func offlineMarket() []MarketEntry {
	var entries []MarketEntry
	json.Unmarshal(offlineMarketJSON, &entries)
	return entries
}

// isGitHubURL 是否 GitHub 域（这些域在国内网络常见不可达，需要代理加速）。
func isGitHubURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "github.com" || host == "raw.githubusercontent.com" ||
		strings.HasSuffix(host, ".githubusercontent.com")
}

// withGitHubProxy 给 GitHub URL 套代理前缀（ghproxy 风格：proxy + 完整原始 URL）。
func (s *Server) withGitHubProxy(raw string) string {
	proxy := s.settings.GitHubProxy()
	if proxy == "" || !isGitHubURL(raw) {
		return raw
	}
	return strings.TrimSuffix(proxy, "/") + "/" + raw
}

// MarketEntry 市场 index.json 的单条目。
// 同名插件以 author+name 判定同插件（name 不保证全局唯一）。
type MarketEntry struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Author      string            `json:"author,omitempty"`
	Label       map[string]string `json:"label"` // 品牌名（多语言，zh 优先）
	PublishedAt string            `json:"published_at,omitempty"`
	DownloadURL string            `json:"download_url"`
	SHA256      string            `json:"sha256"`
}

// marketView 市场条目 + 本机安装状态（前端直接消费）。
type marketView struct {
	MarketEntry
	Installed bool   `json:"installed"`     // 已安装（含同版本）
	Updatable bool   `json:"updatable"`     // 已安装且市场版本更新
	LocalVer  string `json:"local_version"` // 本机已装版本
}

// pluginKey 同插件判定键：author/name（author 缺省回退 name，兼容旧索引）。
func pluginKey(author, name string) string {
	if author == "" {
		return name
	}
	return author + "/" + name
}

// marketplace GET /admin/plugins/marketplace — 线上索引；不可达时回落内置离线清单。
// 每条带本机安装状态（installed/updatable，按 author+name 判定同插件）。
func (s *Server) marketplace(w http.ResponseWriter, r *http.Request) {
	entries, online := s.fetchMarket()
	source := "offline"
	if online {
		source = "online"
	}
	// 本机已装版本（manifest 落盘为准）
	local := map[string]string{}
	for _, name := range s.plugins.Names() {
		if inst, ok := s.plugins.Get(name); ok {
			local[pluginKey(inst.Manifest.Author, inst.Manifest.Name)] = inst.Manifest.Version
		}
	}
	out := make([]marketView, 0, len(entries))
	for _, e := range entries {
		v := marketView{MarketEntry: e, LocalVer: local[pluginKey(e.Author, e.Name)]}
		if v.LocalVer != "" {
			v.Installed = true
			v.Updatable = e.Version != "" && e.Version != v.LocalVer
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"plugins": out, "source": source})
}

// installMarket POST /admin/plugins/install-market — body: {name, author}
// 同插件判定 = author+name（name 不保证全局唯一，与市场列表/已装列表同一判定键）。
func (s *Server) installMarket(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string `json:"name"`
		Author string `json:"author"`
	}
	if !readBody(w, r, &body) || body.Name == "" || body.Author == "" {
		http.Error(w, `{"error":"name and author required"}`, http.StatusBadRequest)
		return
	}
	entries, _ := s.fetchMarket()
	var entry *MarketEntry
	for i := range entries {
		if pluginKey(entries[i].Author, entries[i].Name) == pluginKey(body.Author, body.Name) {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		http.Error(w, `{"error":"not found in marketplace"}`, http.StatusNotFound)
		return
	}

	dl := *entry // 下载 URL 套 GitHub 代理（索引里的原始地址保持干净）
	dl.DownloadURL = s.withGitHubProxy(entry.DownloadURL)
	zipPath, err := downloadToTemp(&dl)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadGateway)
		return
	}
	defer os.Remove(zipPath)
	s.installZipPath(w, r, zipPath)
}

// installUpload POST /admin/plugins/install-upload — multipart 上传 .cphplugin。
func (s *Server) installUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, `{"error":"invalid upload"}`, http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("package")
	if err != nil {
		http.Error(w, `{"error":"missing package field"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "cph-*.cphplugin")
	if err != nil {
		http.Error(w, `{"error":"temp file"}`, http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		http.Error(w, `{"error":"save upload"}`, http.StatusInternalServerError)
		return
	}
	tmp.Close()
	s.installZipPath(w, r, tmp.Name())
}

// installZipPath 安装本地包文件并刷新目录。
func (s *Server) installZipPath(w http.ResponseWriter, r *http.Request, zipPath string) {
	name, err := s.plugins.InstallZip(r.Context(), zipPath)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}
	s.db.Exec(`INSERT OR IGNORE INTO plugins (name, version, author, protocol_version, manifest_json) VALUES (?,?,?,?,?)`, name, "", "cph", 0, "{}")
	s.plugins.RefreshCatalog(r.Context())
	writeJSON(w, http.StatusOK, map[string]interface{}{"installed": name})
}

// stopPlugin POST /admin/plugins/{name}/stop
func (s *Server) stopPlugin(w http.ResponseWriter, r *http.Request) {
	s.plugins.Stop(r.PathValue("name"))
	writeJSON(w, http.StatusOK, map[string]bool{"stopped": true})
}

// startPlugin POST /admin/plugins/{name}/start — 从插件目录重新启动。
func (s *Server) startPlugin(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	bins, err := s.plugins.Scan()
	if err != nil {
		http.Error(w, `{"error":"scan"}`, http.StatusInternalServerError)
		return
	}
	for _, bin := range bins {
		if filepath.Base(filepath.Dir(bin)) == name {
			if _, err := s.plugins.Start(r.Context(), bin); err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
			s.plugins.RefreshCatalog(r.Context())
			writeJSON(w, http.StatusOK, map[string]bool{"started": true})
			return
		}
	}
	http.Error(w, `{"error":"plugin binary not found"}`, http.StatusNotFound)
}

// uninstallPlugin DELETE /admin/plugins/{name} — 停止 + 删除文件 + 清 DB 记录。
func (s *Server) uninstallPlugin(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.plugins.Uninstall(name); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	s.db.Exec("DELETE FROM plugins WHERE name = ?", name)
	s.plugins.RefreshCatalog(r.Context())
	writeJSON(w, http.StatusOK, map[string]bool{"uninstalled": true})
}

// marketURL 当前生效的市场索引地址：用户自建（settings）> config 默认（env / 官方）。
func (s *Server) marketURL() string {
	if u := strings.TrimSpace(s.settings.MarketplaceURL()); u != "" {
		return u
	}
	return s.marketplaceURL
}

// fetchMarket 拉线上市场索引（走 GitHub 代理配置）；不可达时回落内置离线清单。
// online=false 表示返回的是离线兜底。
func (s *Server) fetchMarket() (entries []MarketEntry, online bool) {
	resp, err := marketHTTPClient.Get(s.withGitHubProxy(s.marketURL()))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&entries); err == nil && len(entries) > 0 {
				return entries, true
			}
		}
	}
	return offlineMarket(), false
}

// downloadToTemp 下载市场包（走 GitHub 代理配置）并校验 sha256。
func downloadToTemp(entry *MarketEntry) (string, error) {
	resp, err := marketHTTPClient.Get(entry.DownloadURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "cph-*.cphplugin")
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hasher), resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	tmp.Close()
	if entry.SHA256 != "" {
		got := hex.EncodeToString(hasher.Sum(nil))
		if got != entry.SHA256 {
			os.Remove(tmp.Name())
			return "", fmt.Errorf("sha256 mismatch: want %s got %s", entry.SHA256, got)
		}
	}
	return tmp.Name(), nil
}

var _ = plugin.Manager{} // 保持引用（安装逻辑在 manager 侧）
