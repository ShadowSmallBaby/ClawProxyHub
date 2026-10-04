// system.go — 系统信息 / 备份导出 / 备份导入（导入落到 restore 暂存目录，重启时由 database.ApplyPendingRestore 换入）。
package admin

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/account"
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
	if !requireAdmin(w, r) {
		return
	}
	dir, err := os.MkdirTemp("", "cph-backup-")
	if err != nil {
		http.Error(w, "backup unavailable", 500)
		return
	}
	defer os.RemoveAll(dir)
	key, err := account.BackupKey(s.dataDir)
	if err != nil {
		http.Error(w, "backup key unavailable", 500)
		return
	}
	snapshot := filepath.Join(dir, "cph.db")
	if err = s.db.WithContext(r.Context()).Exec("VACUUM INTO '" + strings.ReplaceAll(filepath.ToSlash(snapshot), "'", "''") + "'").Error; err != nil {
		http.Error(w, "snapshot failed", 500)
		return
	}
	archive, err := os.Create(filepath.Join(dir, "backup.zip"))
	if err != nil {
		http.Error(w, "backup unavailable", 500)
		return
	}
	defer archive.Close()
	zw := zip.NewWriter(archive)
	add := func(name string, reader io.Reader) error {
		wr, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = io.Copy(wr, reader)
		return err
	}
	dbFile, err := os.Open(snapshot)
	if err != nil {
		http.Error(w, "snapshot unavailable", 500)
		return
	}
	err = add("cph.db", dbFile)
	dbFile.Close()
	if err == nil {
		err = add("secret.key", strings.NewReader(string(key)))
	}
	if err == nil {
		meta, _ := json.Marshal(map[string]string{"version": version.Core, "exported_at": time.Now().Format(time.RFC3339)})
		err = add("meta.json", strings.NewReader(string(meta)))
	}
	closeErr := zw.Close()
	if err != nil || closeErr != nil {
		http.Error(w, "backup archive failed", 500)
		return
	}
	if _, err = archive.Seek(0, 0); err != nil {
		http.Error(w, "backup unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="cph-backup-%s.zip"`, time.Now().Format("20060102-150405")))
	io.Copy(w, archive)
}

// importBackup 只发布已校验的独立暂存目录，数据库与可选密钥作为一个集合替换。
func (s *Server) importBackup(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	s.restoreMu.Lock()
	defer s.restoreMu.Unlock()
	r.Body = http.MaxBytesReader(w, r.Body, 512<<20)
	f, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"invalid backup upload"}`, 400)
		return
	}
	defer f.Close()
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	zr, err := zip.NewReader(f, header.Size)
	if err != nil {
		http.Error(w, `{"error":"invalid ZIP"}`, 400)
		return
	}
	if err = os.MkdirAll(s.dataDir, 0700); err != nil {
		http.Error(w, "staging unavailable", 500)
		return
	}
	stage, err := os.MkdirTemp(s.dataDir, ".restore-upload-")
	if err != nil {
		http.Error(w, "staging unavailable", 500)
		return
	}
	defer os.RemoveAll(stage)
	seen := make(map[string]bool)
	fail := func(err error) { http.Error(w, `{"error":"invalid or incompatible backup"}`, http.StatusBadRequest) }
	var total uint64
	for _, zf := range zr.File {
		if zf.UncompressedSize64 > 512<<20 || total > (512<<20)-zf.UncompressedSize64 {
			fail(fmt.Errorf("backup too large"))
			return
		}
		total += zf.UncompressedSize64
		name := zf.Name
		if name != "cph.db" && name != "secret.key" {
			continue
		}
		if seen[name] || zf.Mode()&os.ModeSymlink != 0 {
			fail(fmt.Errorf("duplicate entry"))
			return
		}
		seen[name] = true
		limit := int64(512 << 20)
		if name == "secret.key" {
			limit = 32
		}
		input, e := zf.Open()
		if e != nil {
			fail(e)
			return
		}
		output, e := os.OpenFile(filepath.Join(stage, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			input.Close()
			fail(e)
			return
		}
		n, e := io.Copy(output, io.LimitReader(input, limit+1))
		input.Close()
		if e == nil {
			e = output.Sync()
		}
		ce := output.Close()
		if e != nil || ce != nil || n > limit || (name == "secret.key" && n != 32) {
			fail(fmt.Errorf("invalid archive entry"))
			return
		}
	}
	if !seen["cph.db"] {
		fail(fmt.Errorf("missing database"))
		return
	}
	key, err := account.BackupKey(s.dataDir)
	if err != nil {
		fail(err)
		return
	}
	if seen["secret.key"] {
		incoming, e := os.ReadFile(filepath.Join(stage, "secret.key"))
		if e != nil {
			fail(e)
			return
		}
		if os.Getenv("CPH_SECRET_KEY") != "" && string(incoming) != string(key) {
			fail(fmt.Errorf("CPH_SECRET_KEY mismatch"))
			return
		}
		key = incoming
	}
	if err = database.ValidateBackup(r.Context(), filepath.Join(stage, "cph.db"), key); err != nil {
		fail(err)
		return
	}
	if err = database.PublishRestore(stage, s.dataDir); err != nil {
		http.Error(w, `{"error":"backup not staged"}`, 500)
		return
	}
	writeJSON(w, 200, map[string]interface{}{"ok": true, "with_key": seen["secret.key"], "message": "Backup staged; restart to restore"})
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
