package extension

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

type State struct {
	Manifest  spec.Manifest `json:"manifest"`
	Hash      string        `json:"hash"`
	Signer    string        `json:"signer"`
	Publisher string        `json:"publisher"`
	Enabled   bool          `json:"enabled"`
	Available bool          `json:"available"`
	Status    string        `json:"status"`
	Error     string        `json:"error,omitempty"`
	Bytes     int64         `json:"bytes"`
}
type Manager struct {
	// 操作串行化，生命周期回调不持状态锁，允许回调读取状态。
	op               sync.Mutex
	mu               sync.RWMutex
	dir, hosts, core string
	trust            spec.TrustStore
	fixedTrust       spec.TrustStore
	managedTrust     spec.TrustStore
	saveTrust        func(spec.TrustStore) error
	loadSettings     func(string) (map[string]any, error)
	saveSettings     func(string, map[string]any) error
	states           map[string]State
	activate         func(context.Context, State) error
	deactivate       func(context.Context, string) error
	removeGuard      func(string) error
	validator        func(context.Context, State) error
	prepareUpdate    func(context.Context, State) (UpdateDependents, error)
	management       func(context.Context, spec.Manifest) error
	packageDirs      []string
	sourcesMu        sync.Mutex
	importErrors     map[string]string
	dataHooks        DataHooks
}

// New 共用安装状态，按包类型将运行时和功能扩展存入各自目录。
func New(dataDir, core string, trust spec.TrustStore) *Manager {
	return &Manager{dir: filepath.Join(dataDir, "extensions"), hosts: filepath.Join(dataDir, "hosts"), core: core, trust: copyTrust(trust), fixedTrust: copyTrust(trust), managedTrust: spec.TrustStore{}, states: map[string]State{}}
}

// Inspect 只返回经过宿主信任根验证的包信息，不安装或执行。
func (m *Manager) Inspect(filename string) (*Verified, error) {
	return Verify(filename, m.trustSnapshot())
}
func (m *Manager) SetLifecycle(start func(context.Context, State) error, stop func(context.Context, string) error) {
	m.op.Lock()
	defer m.op.Unlock()
	m.activate = start
	m.deactivate = stop
}
func (m *Manager) SetRemoveGuard(check func(string) error) {
	m.op.Lock()
	defer m.op.Unlock()
	m.removeGuard = check
}

// SetValidator 在安装切换前及启动时检查运行条件，不改变当前激活状态。
func (m *Manager) SetValidator(check func(context.Context, State) error) {
	m.op.Lock()
	defer m.op.Unlock()
	m.validator = check
}

func (m *Manager) validate(ctx context.Context, state State) error {
	if m.validator != nil {
		return m.validator(ctx, clone(state))
	}
	return nil
}
func clone(s State) State {
	data, _ := json.Marshal(s)
	var out State
	_ = json.Unmarshal(data, &out)
	return out
}
func (m *Manager) List() []State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]State, 0, len(m.states))
	for _, s := range m.states {
		out = append(out, clone(s))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.ID < out[j].Manifest.ID })
	return out
}
func (m *Manager) State(id string) (State, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.states[id]
	return clone(s), ok
}
func (m *Manager) put(id string, s *State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s == nil {
		delete(m.states, id)
	} else {
		m.states[id] = clone(*s)
	}
}
func (m *Manager) directory(manifest spec.Manifest) string {
	if manifest.Kind == "runtime" {
		return m.hosts
	}
	return m.dir
}
func archiveName(manifest spec.Manifest) string {
	if manifest.Kind == "runtime" {
		return "package.cphhost"
	}
	return "package.cphext"
}

// Archive 返回安装器保存的原始签名包，平台无需自行拼接路径。
func (m *Manager) Archive(s State) string {
	return filepath.Join(m.directory(s.Manifest), "versions", s.Manifest.ID, s.Hash, archiveName(s.Manifest))
}
func (m *Manager) verified(s State) (*Verified, error) {
	v, err := m.Inspect(m.Archive(s))
	if err != nil {
		return nil, err
	}
	if v.SHA256 != s.Hash || v.Manifest.ID != s.Manifest.ID {
		return nil, fmt.Errorf("stored package identity mismatch")
	}
	return v, nil
}

// Load 在激活前继承旧版停用选择；未安装的身份保留收据，避免自动预装重新启用。
func (m *Manager) Load(ctx context.Context, disabled ...string) error {
	m.op.Lock()
	defer m.op.Unlock()
	if err := os.MkdirAll(m.dir, 0700); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(m.dir, "state.json"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	var saved map[string]State
	if err == nil {
		if err = json.Unmarshal(data, &saved); err != nil {
			return err
		}
	}
	for _, id := range disabled {
		if err := m.remember(id); err != nil {
			return err
		}
	}
	for id, s := range saved {
		if !spec.ValidID(id) || !spec.ValidHash(s.Hash) || s.Manifest.ID != id {
			return fmt.Errorf("invalid extension state")
		}
		migrationErr := m.migrateRuntime(s)
		v, e := m.verified(s)
		if migrationErr != nil {
			e = migrationErr
		}
		s.Available = false
		s.Error = ""
		s.Status = "disabled"
		if contains(disabled, id) {
			s.Enabled = false
		}
		if e != nil {
			s.Status = "invalid"
			s.Error = e.Error()
			s.Enabled = false
		} else {
			s.Manifest = v.Manifest
			s.Signer = v.Signature.KeyID
			s.Publisher = v.Publisher
		}
		m.put(id, &s)
	}
	// 每个包最多尝试一次；等待系统安装的包不会伪装成依赖已可用。
	tried := map[string]bool{}
	for {
		progress := false
		for _, s := range m.List() {
			id := s.Manifest.ID
			if !s.Enabled || tried[id] {
				continue
			}
			if err = m.dependencies(s.Manifest); err != nil {
				continue
			}
			tried[id] = true
			progress = true
			err = m.validate(ctx, s)
			if err == nil {
				err = m.start(ctx, &s)
			}
			if err != nil {
				s.Error = err.Error()
				s.Status = "failed"
			}
			m.put(id, &s)
		}
		if !progress {
			break
		}
	}
	for _, s := range m.List() {
		if s.Enabled && !tried[s.Manifest.ID] {
			s.Status = "blocked"
			s.Error = "dependency unavailable or cycle"
			m.put(s.Manifest.ID, &s)
		}
	}
	return m.persist()
}
func (m *Manager) dependencies(manifest spec.Manifest) error {
	if !manifest.Core.Accepts(m.core) {
		return fmt.Errorf("extension requires another core version")
	}
	if len(manifest.Platforms) > 0 && !contains(manifest.Platforms, runtime.GOOS+"/"+runtime.GOARCH) {
		return fmt.Errorf("extension does not support this platform")
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(spec.Manifest) error
	visit = func(current spec.Manifest) error {
		if visiting[current.ID] {
			return fmt.Errorf("dependency cycle at %s", current.ID)
		}
		if done[current.ID] {
			return nil
		}
		visiting[current.ID] = true
		for id := range current.Dependencies {
			next, ok := m.State(id)
			if id == manifest.ID {
				next.Manifest = manifest
				ok = true
			}
			if ok {
				if err := visit(next.Manifest); err != nil {
					return err
				}
			}
		}
		delete(visiting, current.ID)
		done[current.ID] = true
		return nil
	}
	if err := visit(manifest); err != nil {
		return err
	}
	for id, version := range manifest.Dependencies {
		s, ok := m.State(id)
		if !ok || !version.Accepts(s.Manifest.Version) || !s.Enabled || !s.Available {
			return fmt.Errorf("dependency %s is missing, unavailable or incompatible", id)
		}
	}
	return nil
}
func (m *Manager) start(ctx context.Context, s *State) error {
	s.Enabled = true
	s.Available = false
	s.Error = ""
	if s.Manifest.Activation != "hot" {
		s.Status = "pending-" + s.Manifest.Activation
		return nil
	}
	trustedLua := s.Manifest.ID == "lua-runtime" && s.Manifest.Kind == "runtime" && m.activate != nil && ((s.Manifest.Execution == "trusted-process" && runtime.GOOS != "android") || (s.Manifest.Execution == "android-service" && runtime.GOOS == "android"))
	trustedService := s.Manifest.Kind == "service" && s.Manifest.Backend != nil && m.activate != nil && (runtime.GOOS != "android" || s.Manifest.Backend.Android != nil)
	if s.Manifest.Kind != "frontend-sandbox" && s.Manifest.Kind != "data" && !trustedLua && !trustedService {
		return fmt.Errorf("no isolated executor for %s", s.Manifest.Kind)
	}
	if m.activate != nil {
		if err := m.activate(ctx, clone(*s)); err != nil {
			return err
		}
	}
	s.Available = true
	s.Status = "active"
	return nil
}

// Executable 只交付明确受信的原生入口；普通桌面进程不具备权限沙箱。
func (m *Manager) Executable(s State) (string, error) {
	entry := s.Manifest.Entry
	if s.Manifest.Backend != nil {
		entry = s.Manifest.Backend.Entries[runtime.GOOS+"/"+runtime.GOARCH]
	}
	if (s.Manifest.Execution != "trusted-process" && s.Manifest.Execution != "android-service") || !spec.SafePath(entry) {
		return "", fmt.Errorf("not a trusted runtime")
	}
	v, err := m.verified(s)
	if err != nil {
		return "", err
	}
	path := filepath.Join(m.directory(s.Manifest), "versions", s.Manifest.ID, s.Hash, filepath.FromSlash(entry))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(data, v.Files[entry]) {
		return "", fmt.Errorf("executable hash mismatch")
	}
	if s.Manifest.Backend != nil {
		info, err := buildinfo.Read(bytes.NewReader(data))
		if err != nil {
			return "", fmt.Errorf("backend is not a Go executable: %w", err)
		}
		settings := map[string]string{}
		for _, setting := range info.Settings {
			settings[setting.Key] = setting.Value
		}
		cgo, buildMode := "0", "exe"
		if runtime.GOOS == "android" {
			if s.Manifest.Backend.Android == nil {
				return "", fmt.Errorf("backend does not declare an Android executor")
			}
			cgo, buildMode = "1", "c-shared"
		}
		if settings["GOOS"] != runtime.GOOS || settings["GOARCH"] != runtime.GOARCH || settings["CGO_ENABLED"] != cgo || settings["-buildmode"] != buildMode {
			return "", fmt.Errorf("backend platform or CGO mode is incompatible")
		}
	}
	mode := fs.FileMode(0700)
	if runtime.GOOS == "android" {
		mode = 0500
	}
	if err = os.Chmod(path, mode); err != nil {
		return "", err
	}
	return filepath.Abs(path)
}
func (m *Manager) stop(ctx context.Context, s State) error {
	if (s.Available || s.Status == "stopping") && m.deactivate != nil {
		err := m.deactivate(ctx, s.Manifest.ID)
		if err != nil && s.Manifest.Kind != "runtime" {
			// 动作注销可能已取消请求，不能继续报告该扩展可用。
			s.Available = false
			s.Status = "stopping"
			s.Error = err.Error()
			m.put(s.Manifest.ID, &s)
			return errors.Join(err, m.persist())
		}
		return err
	}
	return nil
}
func (m *Manager) persist() error {
	m.mu.RLock()
	data, err := json.MarshalIndent(m.states, "", "  ")
	m.mu.RUnlock()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(m.dir, ".state-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	err = errors.Join(err, tmp.Close())
	if err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(m.dir, "state.json")); err != nil {
		return err
	}
	if m.dataHooks.Committed != nil {
		return m.dataHooks.Committed(m.List())
	}
	return nil
}
func (m *Manager) restore(ctx context.Context, old State, exists bool) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if !exists {
		m.put(old.Manifest.ID, nil)
		return nil
	}
	var err error
	if old.Available {
		err = m.start(ctx, &old)
		if err != nil {
			old.Available = false
			old.Status = "failed"
			old.Error = "rollback: " + err.Error()
		}
	}
	m.put(old.Manifest.ID, &old)
	return err
}
func (m *Manager) Install(ctx context.Context, filename string, grant []string) (State, error) {
	return m.InstallExpected(ctx, filename, grant, "")
}

// InstallExpected 将预装锁定哈希与同一校验快照绑定，避免两次读取间替换包。
func (m *Manager) InstallExpected(ctx context.Context, filename string, grant []string, expected string) (State, error) {
	return m.install(ctx, filename, grant, expected, false)
}

func (m *Manager) install(ctx context.Context, filename string, grant []string, expected string, initial bool) (State, error) {
	m.op.Lock()
	defer m.op.Unlock()
	v, err := m.Inspect(filename)
	if err != nil {
		return State{}, err
	}
	if expected != "" && v.SHA256 != expected {
		return State{}, fmt.Errorf("package differs from locked hash")
	}
	manifest := v.Manifest
	old, exists := m.State(manifest.ID)
	// 首次预装在操作锁内复核，避免扫描后发生安装或卸载时覆盖用户选择。
	if initial {
		if exists {
			return old, nil
		}
		if remembered, err := m.remembered(manifest.ID); err != nil || remembered {
			return State{}, err
		}
	}
	if err = m.CheckManagement(ctx, manifest); err != nil {
		return State{}, err
	}
	if manifest.Target != "backend" {
		return State{}, fmt.Errorf("package targets the client; install through the platform client")
	}
	if err = m.dependencies(manifest); err != nil {
		return State{}, err
	}
	if exists {
		if old.Signer != v.Signature.KeyID {
			return State{}, fmt.Errorf("signer change requires explicit trust migration")
		}
		if old.Hash == v.SHA256 {
			return old, nil
		}
		if c, _ := spec.Compare(manifest.Version, old.Manifest.Version); c < 0 {
			return State{}, fmt.Errorf("extension downgrades are not supported")
		}
		for _, dependent := range m.List() {
			if r, ok := dependent.Manifest.Dependencies[manifest.ID]; ok && dependent.Enabled && !r.Accepts(manifest.Version) {
				return State{}, fmt.Errorf("upgrade conflicts with %s", dependent.Manifest.ID)
			}
		}
	}
	for _, p := range manifest.Permissions {
		if !contains(grant, p) {
			return State{}, fmt.Errorf("permission %q requires authorization", p)
		}
	}
	size, err := m.store(v)
	if err != nil {
		return State{}, err
	}
	next := State{Manifest: manifest, Hash: v.SHA256, Signer: v.Signature.KeyID, Publisher: v.Publisher, Enabled: true, Bytes: size}
	if err = m.validate(ctx, next); err != nil {
		return State{}, err
	}
	var dependents UpdateDependents
	if exists && old.Available && m.prepareUpdate != nil {
		dependents, err = m.prepareUpdate(ctx, clone(old))
		if err != nil {
			return State{}, err
		}
		if dependents != nil {
			defer dependents.Close()
		}
	}
	if err = m.stop(ctx, old); err != nil {
		return State{}, errors.Join(err, resumeDependents(ctx, dependents))
	}
	// 停用状态的更新也提交表结构，但不启动扩展程序。
	if (exists && !old.Enabled || manifest.Activation != "hot") && m.dataHooks.Prepare != nil {
		if err = m.dataHooks.Prepare(ctx, clone(next)); err != nil {
			return State{}, errors.Join(err, m.rollbackUpdate(ctx, next, old, exists, false, dependents))
		}
	}
	if exists && !old.Enabled {
		next.Enabled, next.Status = false, "disabled"
	} else if err = m.start(ctx, &next); err != nil {
		return State{}, errors.Join(err, m.rollbackUpdate(ctx, next, old, exists, false, dependents))
	}
	m.put(manifest.ID, &next)
	// 平台连接器从共享状态文件读取运行时，恢复插件前必须先写入候选状态。
	if err = m.persist(); err != nil {
		return State{}, errors.Join(err, m.rollbackUpdate(ctx, next, old, exists, false, dependents))
	}
	if dependents != nil {
		if err = dependents.Resume(ctx); err != nil {
			return State{}, errors.Join(err, m.rollbackUpdate(ctx, next, old, exists, true, dependents))
		}
	}
	return next, nil
}
func (m *Manager) SetEnabled(ctx context.Context, id string, enabled bool) error {
	m.op.Lock()
	defer m.op.Unlock()
	s, ok := m.State(id)
	if !ok {
		return fmt.Errorf("extension not installed")
	}
	old := s
	if err := m.CheckManagement(ctx, s.Manifest); err != nil {
		return err
	}
	var dependents UpdateDependents
	if enabled {
		if s.Status == "stopping" {
			if err := m.stop(ctx, s); err != nil {
				return err
			}
		}
		if err := m.dependencies(s.Manifest); err != nil {
			return err
		}
		if _, err := m.verified(s); err != nil {
			return err
		}
		if s.Available {
			return nil
		}
		if err := m.validate(ctx, s); err != nil {
			return err
		}
		if err := m.start(ctx, &s); err != nil {
			return err
		}
	} else {
		if !s.Enabled && !s.Available && s.Status == "disabled" {
			return nil
		}
		if err := m.canRemove(id); err != nil {
			return err
		}
		if s.Available && m.prepareUpdate != nil {
			var err error
			dependents, err = m.prepareUpdate(ctx, clone(s))
			if err != nil {
				return err
			}
			if dependents != nil {
				defer dependents.Close()
			}
		}
		if err := m.stop(ctx, s); err != nil {
			return errors.Join(err, resumeDependents(ctx, dependents))
		}
		s.Enabled = false
		s.Available = false
		s.Status = "disabled"
		s.Error = ""
	}
	m.put(id, &s)
	if err := m.persist(); err != nil {
		return errors.Join(err, m.rollbackUpdate(ctx, s, old, true, false, dependents))
	}
	return nil
}
func (m *Manager) canRemove(id string) error {
	if m.removeGuard != nil {
		if err := m.removeGuard(id); err != nil {
			return err
		}
	}
	for _, s := range m.List() {
		if _, ok := s.Manifest.Dependencies[id]; ok && s.Enabled {
			return fmt.Errorf("required by enabled extension %s", s.Manifest.ID)
		}
	}
	return nil
}
func (m *Manager) Uninstall(ctx context.Context, id string) error {
	m.op.Lock()
	defer m.op.Unlock()
	if !spec.ValidID(id) {
		return fmt.Errorf("invalid extension ID")
	}
	old, ok := m.State(id)
	if !ok {
		return fs.ErrNotExist
	}
	if err := m.CheckManagement(ctx, old.Manifest); err != nil {
		return err
	}
	if err := m.canRemove(id); err != nil {
		return err
	}
	if err := m.remember(id); err != nil {
		return err
	}
	if err := m.stop(ctx, old); err != nil {
		return err
	}
	m.put(id, nil)
	if err := m.persist(); err != nil {
		return errors.Join(err, m.restore(ctx, old, true))
	}
	// 先提交卸载事实再删除可重建代码，用户数据仍由独立操作清理。
	if err := os.RemoveAll(filepath.Join(m.directory(old.Manifest), "versions", id)); err != nil {
		return fmt.Errorf("extension uninstalled; clean remaining cache: %w", err)
	}
	return nil
}

// CleanData 只允许卸载后显式清理，卸载本身保留数据。
func (m *Manager) CleanData(ctx context.Context, id string) error {
	m.op.Lock()
	defer m.op.Unlock()
	if !spec.ValidID(id) {
		return fmt.Errorf("invalid extension ID")
	}
	if _, ok := m.State(id); ok {
		return fmt.Errorf("uninstall extension before clearing data")
	}
	if err := m.CheckManagement(ctx, spec.Manifest{ID: id}); err != nil {
		return err
	}
	if m.dataHooks.Clear != nil {
		if err := m.dataHooks.Clear(ctx, id); err != nil {
			return err
		}
	}
	if err := errors.Join(os.RemoveAll(filepath.Join(m.dir, "data", id)), os.RemoveAll(filepath.Join(m.hosts, "data", id))); err != nil {
		return err
	}
	if m.saveSettings != nil {
		return m.saveSettings(id, nil)
	}
	return nil
}
func (m *Manager) CleanCache(ctx context.Context, id string) error {
	m.op.Lock()
	defer m.op.Unlock()
	if !spec.ValidID(id) {
		return fmt.Errorf("invalid extension ID")
	}
	s, _ := m.State(id)
	s.Manifest.ID = id
	if err := m.CheckManagement(ctx, s.Manifest); err != nil {
		return err
	}
	for _, directory := range []string{m.dir, m.hosts} {
		base := filepath.Join(directory, "versions", id)
		entries, err := os.ReadDir(base)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		for _, e := range entries {
			if directory != m.directory(s.Manifest) || e.Name() != s.Hash {
				if err = os.RemoveAll(filepath.Join(base, e.Name())); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ReadAsset 在交付前重新校验实际文件，阻止解压目录被篡改后绕过签名。
func (m *Manager) ReadAsset(id, hash, name string) (*bytes.Reader, error) {
	s, ok := m.State(id)
	if !ok || !s.Available || s.Hash != hash || s.Manifest.Files[name] == "" || !spec.SafePath(name) {
		return nil, fs.ErrNotExist
	}
	if s.Manifest.Backend != nil {
		if strings.HasPrefix(name, "backend/") {
			return nil, fs.ErrNotExist
		}
		for _, entry := range s.Manifest.Backend.Entries {
			if entry == name {
				return nil, fs.ErrNotExist
			}
		}
	}
	root, err := os.OpenRoot(filepath.Join(m.directory(s.Manifest), "versions", id, hash))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxPackageBytes+1))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != s.Manifest.Files[name] {
		return nil, fmt.Errorf("installed asset hash mismatch")
	}
	return bytes.NewReader(data), nil
}
func (m *Manager) Close(ctx context.Context) error {
	m.op.Lock()
	defer m.op.Unlock()
	var err error
	states := m.List()
	visited := map[string]bool{}
	var stop func(State)
	stop = func(s State) {
		if visited[s.Manifest.ID] {
			return
		}
		visited[s.Manifest.ID] = true
		for _, d := range states {
			if _, ok := d.Manifest.Dependencies[s.Manifest.ID]; ok {
				stop(d)
			}
		}
		err = errors.Join(err, m.stop(ctx, s))
		s.Available = false
		m.put(s.Manifest.ID, &s)
	}
	for _, s := range states {
		stop(s)
	}
	return err
}
