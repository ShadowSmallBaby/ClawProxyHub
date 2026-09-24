// marketplace.go — 插件市场与包安装。
package admin

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/plugin"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
)

// 离线插件市场索引（线上索引不可达时的兜底清单）
// 内容是 ClawProxyHubPlugins 仓库 index.json 的快照，随核心发布同步；sha256 为空表示跳过校验。
//
//go:embed offline_market.json
var offlineMarketJSON []byte

// marketHTTPClient 市场索引请求（小 JSON）：整体 20s 超时，网络不通时快速失败。
var marketHTTPClient = &http.Client{Timeout: 20 * time.Second}

// downloadHTTPClient 插件包下载（可能几十 MB 走 GitHub 代理）：不设整体超时，
// 只在连接 / 响应头 / 空闲阶段设限——慢速大包不被整体 timeout 砍断（升级超时根因）。
var downloadHTTPClient = &http.Client{
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	},
}

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
	Source      string            `json:"source,omitempty"` // 来源插件源名（聚合时由核心填入，索引里不含）
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

// marketplace GET /admin/plugins/marketplace?source= — 指定源只拉该源（前端按源懒加载）；
// 不指定则聚合全部启用源。一个源都不可达时回落内置离线清单。
// 每条带来源与本机安装状态（installed/updatable，按 author+name 判定同插件）。
func (s *Server) marketplace(w http.ResponseWriter, r *http.Request) {
	entries, online := s.fetchMarket(r.URL.Query().Get("source"))
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

// installMarket POST /admin/plugins/install-market — body: {name, author, source?}
// 同插件判定 = author+name（name 不保证全局唯一，与市场列表/已装列表同一判定键）；
// source 指定来源（多源同名时必填），缺省取第一个匹配。
// 响应是 NDJSON 进度流（下载可能走 GitHub 代理、耗时不定）：
// {phase:"downloading",received,total} → {phase:"installing"} → {phase:"starting"} → {installed} 或 {error}。
func (s *Server) installMarket(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string `json:"name"`
		Author string `json:"author"`
		Source string `json:"source"`
	}
	if !readBody(w, r, &body) || body.Name == "" || body.Author == "" {
		http.Error(w, `{"error":"name and author required"}`, http.StatusBadRequest)
		return
	}
	entries, _ := s.fetchMarket(body.Source)
	var entry *MarketEntry
	for i := range entries {
		if pluginKey(entries[i].Author, entries[i].Name) != pluginKey(body.Author, body.Name) {
			continue
		}
		if body.Source == "" || entries[i].Source == body.Source {
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
	pw := newProgressWriter(w)
	// r.Context() 随客户端断开而取消：前端点「取消」abort fetch → 连接断 → 下载中断
	zipPath, err := downloadToTemp(r.Context(), &dl, func(received, total int64) {
		pw.send(map[string]interface{}{"phase": "downloading", "received": received, "total": total})
	})
	if err != nil {
		pw.send(map[string]string{"error": err.Error()})
		return
	}
	defer os.Remove(zipPath)
	name, err := s.installZip(r.Context(), zipPath, entry.Source, func(phase string) {
		pw.send(map[string]string{"phase": phase})
	})
	if err != nil {
		pw.send(map[string]string{"error": err.Error()})
		return
	}
	pw.send(map[string]string{"installed": name})
}

// progressWriter NDJSON 进度流：每行一个 JSON 事件，写后即刷（前端逐行渲染）。
type progressWriter struct {
	w  http.ResponseWriter
	fl http.Flusher
}

func newProgressWriter(w http.ResponseWriter) *progressWriter {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // 反代不缓冲
	w.WriteHeader(http.StatusOK)
	fl, _ := w.(http.Flusher)
	return &progressWriter{w: w, fl: fl}
}

func (p *progressWriter) send(v interface{}) {
	json.NewEncoder(p.w).Encode(v) // Encode 自带换行
	if p.fl != nil {
		p.fl.Flush()
	}
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
	name, err := s.installZip(r.Context(), tmp.Name(), "", nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"installed": name})
}

// installZip 安装本地包文件并刷新目录（市场 / 上传共用），onPhase 透传给 InstallZip 报进度。
// source 为来源插件源名：官方源/手动上传为空装在根目录，其他源按源名建命名空间目录；
// 同名插件已从其他来源安装时拒绝（插件身份 = manifest.name，目录隔离不改变身份唯一性）。
func (s *Server) installZip(ctx context.Context, zipPath, source string, onPhase func(string)) (string, error) {
	if source == setting.OfficialSourceName {
		source = ""
	}
	name, err := s.plugins.InstallZip(ctx, zipPath, source, onPhase)
	if err != nil {
		return "", err
	}
	s.db.Exec(`INSERT OR IGNORE INTO plugins (name, version, author, protocol_version, manifest_json, source) VALUES (?,?,?,?,?,?)`, name, "", "cph", 0, "{}", source)
	s.db.Exec(`UPDATE plugins SET source = ? WHERE name = ?`, source, name)
	s.plugins.RefreshCatalog(ctx)
	return name, nil
}

// stopPlugin POST /admin/plugins/{name}/stop — 停止并写持久化状态（重启核心保持停止）。
func (s *Server) stopPlugin(w http.ResponseWriter, r *http.Request) {
	s.plugins.Stop(r.PathValue("name"), true)
	writeJSON(w, http.StatusOK, map[string]bool{"stopped": true})
}

// startPlugin POST /admin/plugins/{name}/start — 从插件目录重新启动（已在运行则直接返回）；
// 成功即清除持久化停止状态（下次重启核心照常拉起）。
func (s *Server) startPlugin(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := s.plugins.Get(name); ok {
		writeJSON(w, http.StatusOK, map[string]bool{"started": true})
		return
	}
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
			s.plugins.Resume(name)
			s.plugins.RefreshCatalog(r.Context())
			writeJSON(w, http.StatusOK, map[string]bool{"started": true})
			return
		}
	}
	http.Error(w, `{"error":"plugin binary not found"}`, http.StatusNotFound)
}

// uninstallPlugin DELETE /admin/plugins/{name} — 停止 + 删除文件 + 级联清 DB（实例/分组/账号/任务）。
func (s *Server) uninstallPlugin(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.plugins.Uninstall(name); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	impact := deleteImpact{Routes: []string{}, Keys: []string{}}
	var p model.Plugin
	if err := s.db.Where("name = ?", name).First(&p).Error; err == nil {
		sc := scopePlugin(s.db, p.ID)
		impact = s.impact(sc)
		if err := s.cascadeDelete(sc); err != nil {
			http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
			return
		}
		s.db.Delete(&p)
	}
	s.db.Where("plugin = ?", name).Delete(&model.PluginStore{}) // 清该插件的 KV 状态
	s.plugins.RefreshCatalog(r.Context())
	writeJSON(w, http.StatusOK, map[string]interface{}{"uninstalled": true, "impact": impact})
}

// enabledSources 启用的插件源（一个都没有时退回 config 默认官方地址，避免误配把市场关死）。
func (s *Server) enabledSources() []setting.PluginSource {
	var out []setting.PluginSource
	for _, src := range s.settings.PluginSources() {
		if src.Enabled && strings.TrimSpace(src.URL) != "" {
			out = append(out, src)
		}
	}
	if len(out) == 0 {
		out = []setting.PluginSource{{Name: setting.OfficialSourceName, URL: s.marketplaceURL, Enabled: true}}
	}
	return out
}

// fetchMarket 拉插件索引（走 GitHub 代理配置），每条标记来源；
// only 非空只拉该源，否则聚合全部启用源。一个源都拉不到时回落内置离线清单（仅官方源语义），online=false。
func (s *Server) fetchMarket(only string) (entries []MarketEntry, online bool) {
	for _, src := range s.enabledSources() {
		if only != "" && src.Name != only {
			continue
		}
		list, err := fetchIndex(s.withGitHubProxy(strings.TrimSpace(src.URL)))
		if err != nil {
			continue
		}
		for i := range list {
			list[i].Source = src.Name
		}
		entries = append(entries, list...)
		online = true
	}
	if !online && (only == "" || only == setting.OfficialSourceName) {
		entries = offlineMarket()
		for i := range entries {
			entries[i].Source = setting.OfficialSourceName
		}
	}
	return entries, online
}

// fetchIndex 拉一个源的 index.json。
func fetchIndex(url string) ([]MarketEntry, error) {
	resp, err := marketHTTPClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var list []MarketEntry
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	return list, nil
}

// listPluginSources GET /admin/plugin-sources — 每个源附带实时条目数与本机已装数（源不可达为 0/reachable=false）。
func (s *Server) listPluginSources(w http.ResponseWriter, r *http.Request) {
	local := map[string]bool{}
	for _, name := range s.plugins.Names() {
		if inst, ok := s.plugins.Get(name); ok {
			local[pluginKey(inst.Manifest.Author, inst.Manifest.Name)] = true
		}
	}
	type sourceView struct {
		setting.PluginSource
		PluginCount    int  `json:"plugin_count"`
		InstalledCount int  `json:"installed_count"`
		Reachable      bool `json:"reachable"`
	}
	srcs := s.settings.PluginSources()
	out := make([]sourceView, len(srcs))
	for i, src := range srcs {
		v := sourceView{PluginSource: src}
		list, err := fetchIndex(s.withGitHubProxy(strings.TrimSpace(src.URL)))
		if err != nil && src.Name == setting.OfficialSourceName {
			list, err = offlineMarket(), nil // 官方源不可达用离线快照计数
		}
		if err == nil {
			v.Reachable = true
			v.PluginCount = len(list)
			for _, e := range list {
				if local[pluginKey(e.Author, e.Name)] {
					v.InstalledCount++
				}
			}
		}
		out[i] = v
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"sources": out})
}

// probePluginSource GET /admin/plugin-sources/probe?url= — 试拉一个索引地址，返回条目数（添加/编辑源时校验可达）。
func (s *Server) probePluginSource(w http.ResponseWriter, r *http.Request) {
	u := strings.TrimSpace(r.URL.Query().Get("url"))
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		http.Error(w, `{"error":"源地址需以 http:// 或 https:// 开头（指向 index.json）"}`, http.StatusBadRequest)
		return
	}
	list, err := fetchIndex(s.withGitHubProxy(u))
	if err != nil {
		http.Error(w, `{"error":"索引不可达或格式无效: `+err.Error()+`"}`, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"plugin_count": len(list)})
}

// putPluginSources PUT /admin/plugin-sources — body: {sources: [{name,url,enabled}]} 全量替换。
// 源名用作命名空间目录名：仅允许字母数字 - _，且不可重复；至少保留 official。
func (s *Server) putPluginSources(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Sources []setting.PluginSource `json:"sources"`
	}
	if !readBody(w, r, &body) {
		return
	}
	seen := map[string]bool{}
	for i := range body.Sources {
		src := &body.Sources[i]
		src.Name = strings.TrimSpace(src.Name)
		src.URL = strings.TrimSpace(src.URL)
		if !validSourceName(src.Name) {
			http.Error(w, `{"error":"源名仅允许字母、数字、- 与 _（1–32 位）"}`, http.StatusBadRequest)
			return
		}
		if seen[src.Name] {
			http.Error(w, `{"error":"源名重复: `+src.Name+`"}`, http.StatusBadRequest)
			return
		}
		seen[src.Name] = true
		if !strings.HasPrefix(src.URL, "http://") && !strings.HasPrefix(src.URL, "https://") {
			http.Error(w, `{"error":"源地址需以 http:// 或 https:// 开头（指向 index.json）"}`, http.StatusBadRequest)
			return
		}
	}
	if !seen[setting.OfficialSourceName] {
		http.Error(w, `{"error":"必须保留 official 源（可停用但不可删除）"}`, http.StatusBadRequest)
		return
	}
	s.settings.SetPluginSources(body.Sources)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// validSourceName 源名同时是磁盘目录名，限制字符集防路径穿越。
func validSourceName(name string) bool {
	if name == "" || len(name) > 32 {
		return false
	}
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// downloadToTemp 下载市场包（走 GitHub 代理配置）并校验 sha256；report 按进度回调（total 未知为 -1）。
// ctx 取消（客户端断开）时 Body 读取即刻中断，用于「取消安装」。
func downloadToTemp(ctx context.Context, entry *MarketEntry, report func(received, total int64)) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", entry.DownloadURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := downloadHTTPClient.Do(req)
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
	src := &progressReader{r: resp.Body, total: resp.ContentLength, report: report}
	if _, err := io.Copy(io.MultiWriter(tmp, hasher), src); err != nil {
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

// progressReader 计数读取器：每 256KB 或读完时回调一次，避免进度事件刷屏。
type progressReader struct {
	r                  io.Reader
	total              int64
	received, lastSent int64
	report             func(received, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.received += int64(n)
	if p.report != nil && (p.received-p.lastSent >= 256<<10 || err != nil) {
		p.lastSent = p.received
		p.report(p.received, p.total)
	}
	return n, err
}

var _ = plugin.Manager{} // 保持引用（安装逻辑在 manager 侧）
