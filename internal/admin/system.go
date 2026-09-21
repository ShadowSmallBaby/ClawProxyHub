// system.go — 系统信息 / 备份导出 / 备份导入（导入落到 restore 暂存目录，重启时由 database.ApplyPendingRestore 换入）。
package admin

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/database"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/version"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
)

// startedAt 进程启动时刻（运行时长）。
var startedAt = time.Now()

// routeSystem 系统端点（备份含凭据密钥，导出/导入均限 admin）。
func (s *Server) routeSystem(r authed) {
	r.h("GET /admin/system/info", s.systemInfo)
	r.h("GET /admin/system/backup", s.exportBackup)
	r.h("POST /admin/system/restore", s.importBackup)
}

// systemInfo GET /admin/system/info — 版本 / 运行时 / 路径 / 库体积 / 各表计数。
func (s *Server) systemInfo(w http.ResponseWriter, r *http.Request) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	var dbSize int64
	if st, err := os.Stat(s.dbPath); err == nil {
		dbSize = st.Size()
	}
	migVer, _, _ := database.CachedMigrationVersion()
	counts := map[string]int64{}
	for name, m := range map[string]interface{}{
		"plugins": &model.Plugin{}, "instances": &model.Instance{}, "accounts": &model.Account{},
		"groups": &model.Group{}, "routes": &model.Route{}, "keys": &model.Key{},
		"request_logs": &model.RequestLog{}, "task_runs": &model.TaskRun{}, "notifications": &model.Notification{},
	} {
		var n int64
		s.db.Model(m).Count(&n)
		counts[name] = n
	}
	pendingRestore := false
	if _, err := os.Stat(filepath.Join(s.dataDir, "restore", "cph.db")); err == nil {
		pendingRestore = true
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":           version.Core,
		"protocol_version":  sdk.ProtocolVersion,
		"go_version":        runtime.Version(),
		"os":                runtime.GOOS,
		"arch":              runtime.GOARCH,
		"started_at":        startedAt.Format(time.RFC3339),
		"uptime_seconds":    int64(time.Since(startedAt).Seconds()),
		"data_dir":          absPath(s.dataDir),
		"db_size_bytes":     dbSize,
		"migration_version": migVer,
		"mem_alloc_bytes":   ms.Alloc,
		"goroutines":        runtime.NumGoroutine(),
		"counts":            counts,
		"pending_restore":   pendingRestore,
	})
}

func absPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

// exportBackup GET /admin/system/backup — zip：cph.db（VACUUM INTO 一致性快照）+ secret.key（凭据加解密密钥）。
func (s *Server) exportBackup(w http.ResponseWriter, r *http.Request) {
	if roleOf(r) != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	tmp, err := os.CreateTemp("", "cph-backup-*.db")
	if err != nil {
		http.Error(w, `{"error":"temp file"}`, http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	tmp.Close()
	os.Remove(tmpPath) // VACUUM INTO 要求目标不存在
	defer os.Remove(tmpPath)
	// 单引号转义路径（VACUUM INTO 不接受绑定参数的实现兼容）
	if err := s.db.Exec("VACUUM INTO '" + strings.ReplaceAll(filepath.ToSlash(tmpPath), "'", "''") + "'").Error; err != nil {
		http.Error(w, `{"error":"snapshot failed: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="cph-backup-%s.zip"`, time.Now().Format("20060102-150405")))
	zw := zip.NewWriter(w)
	defer zw.Close()
	addFile := func(name, src string) error {
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		defer f.Close()
		st, _ := f.Stat()
		hdr := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()}
		if st != nil {
			hdr.Modified = st.ModTime()
		}
		wr, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		_, err = io.Copy(wr, f)
		return err
	}
	if err := addFile("cph.db", tmpPath); err != nil {
		return
	}
	if key := filepath.Join(s.dataDir, "secret.key"); fileExists(key) {
		addFile("secret.key", key)
	}
	meta := fmt.Sprintf(`{"version":"%s","exported_at":"%s"}`, version.Core, time.Now().Format(time.RFC3339))
	if wr, err := zw.Create("meta.json"); err == nil {
		wr.Write([]byte(meta))
	}
}

// importBackup POST /admin/system/restore — multipart file=<zip>；校验后写入 <data>/restore/，重启时换入。
func (s *Server) importBackup(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 512<<20)
	f, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"请选择备份 zip 文件"}`, http.StatusBadRequest)
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, `{"error":"读取上传失败"}`, http.StatusBadRequest)
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		http.Error(w, `{"error":"不是合法的 zip 备份"}`, http.StatusBadRequest)
		return
	}
	files := map[string][]byte{}
	for _, zf := range zr.File {
		name := filepath.Base(zf.Name)
		if name != "cph.db" && name != "secret.key" {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			continue
		}
		files[name], _ = io.ReadAll(rc)
		rc.Close()
	}
	dbRaw, ok := files["cph.db"]
	if !ok || !bytes.HasPrefix(dbRaw, []byte("SQLite format 3\x00")) {
		http.Error(w, `{"error":"备份包缺少 cph.db 或不是 SQLite 数据库"}`, http.StatusBadRequest)
		return
	}
	dir := filepath.Join(s.dataDir, "restore")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, `{"error":"创建暂存目录失败"}`, http.StatusInternalServerError)
		return
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
			http.Error(w, `{"error":"写入暂存失败: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "with_key": files["secret.key"] != nil,
		"message": "备份已暂存，重启服务后自动换入（当前库会备份为 cph.db.bak-<时间>）",
	})
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
