// Package extension 定义跨平台扩展包与受控动作贡献，不沿用业务插件协议版本。
package extension

import (
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
	"strings"
)

const APIVersion = 1

type VersionRange struct {
	Min          string `json:"min,omitempty"`
	MaxExclusive string `json:"max_exclusive,omitempty"`
}
type Page struct {
	ID     string            `json:"id"`
	Title  string            `json:"title"`
	Labels map[string]string `json:"labels,omitempty"`
	Entry  string            `json:"entry"`
}
type Contribution struct {
	ID       string            `json:"id"`
	Location string            `json:"location"`
	Label    string            `json:"label"`
	Labels   map[string]string `json:"labels,omitempty"`
	Page     string            `json:"page"`
	When     string            `json:"when,omitempty"`
	Order    int               `json:"order,omitempty"`
}
type Action struct {
	ID         string            `json:"id"`
	Target     string            `json:"target,omitempty"`
	Title      string            `json:"title"`
	Labels     map[string]string `json:"labels,omitempty"`
	Permission string            `json:"permission,omitempty"`
	Effect     string            `json:"effect,omitempty"`
	TimeoutMS  int               `json:"timeout_ms,omitempty"`
	Input      *Schema           `json:"input_schema,omitempty"`
	Output     *Schema           `json:"output_schema,omitempty"`
}
type Manifest struct {
	ID            string                  `json:"id"`
	Version       string                  `json:"version"`
	Name          string                  `json:"name"`
	Label         map[string]string       `json:"label,omitempty"`
	Desc          map[string]string       `json:"desc,omitempty"`
	Kind          string                  `json:"kind"`
	API           int                     `json:"api"`
	Core          VersionRange            `json:"core"`
	Dependencies  map[string]VersionRange `json:"dependencies,omitempty"`
	Permissions   []string                `json:"permissions"`
	Capabilities  []string                `json:"capabilities,omitempty"`
	Platforms     []string                `json:"platforms,omitempty"`
	Environments  []string                `json:"environments,omitempty"`
	Target        string                  `json:"target"`
	Activation    string                  `json:"activation"`
	Execution     string                  `json:"execution,omitempty"`
	Entry         string                  `json:"entry,omitempty"`
	Pages         []Page                  `json:"pages,omitempty"`
	Contributions []Contribution          `json:"contributions,omitempty"`
	Actions       []Action                `json:"actions,omitempty"`
	Settings      []Setting               `json:"settings,omitempty"`
	Files         map[string]string       `json:"files"`
	Android       *AndroidRuntime         `json:"android,omitempty"`
	Backend       *Backend                `json:"backend,omitempty"`
	Storage       *StorageSchema          `json:"storage,omitempty"`
}

// AndroidRuntime 描述由应用私有服务加载的运行时库。
type AndroidRuntime struct {
	Format  string            `json:"format"`
	Library string            `json:"library"`
	ABI     string            `json:"abi"`
	MinSDK  int               `json:"min_sdk"`
	Files   map[string]string `json:"files"`
}
type Signature struct {
	KeyID       string `json:"key_id"`
	Value       string `json:"value"`
	Algorithm   string `json:"algorithm,omitempty"`
	Certificate string `json:"certificate,omitempty"`
}

// UnmarshalJSON 让签名清单中的未知能力直接失败，避免静默忽略权限字段。
func (m *Manifest) UnmarshalJSON(raw []byte) error {
	type plain Manifest
	return DecodeStrict(raw, (*plain)(m))
}

type TrustKey struct {
	PublicKey   string   `json:"public_key,omitempty"`
	Certificate string   `json:"certificate,omitempty"`
	Publisher   string   `json:"publisher"`
	IDs         []string `json:"ids"`
	Permissions []string `json:"permissions"`
	Native      bool     `json:"native,omitempty"`
}
type TrustStore map[string]TrustKey

var identifier = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)

func ValidID(id string) bool { return len(id) <= 96 && identifier.MatchString(id) }
func SafePath(name string) bool {
	if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, `\:`) {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
		base := strings.ToUpper(strings.Split(part, ".")[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || regexp.MustCompile(`^(COM|LPT)[1-9]$`).MatchString(base) {
			return false
		}
	}
	return true
}
func Version(value string) ([3]uint64, error) {
	var result [3]uint64
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return result, fmt.Errorf("expected stable major.minor.patch version: %q", value)
	}
	for i, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return result, fmt.Errorf("invalid version %q", value)
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return result, err
		}
		result[i] = n
	}
	return result, nil
}
func Compare(a, b string) (int, error) {
	x, err := Version(a)
	if err != nil {
		return 0, err
	}
	y, err := Version(b)
	if err != nil {
		return 0, err
	}
	for i := range x {
		if x[i] < y[i] {
			return -1, nil
		}
		if x[i] > y[i] {
			return 1, nil
		}
	}
	return 0, nil
}
func (r VersionRange) Accepts(value string) bool {
	if _, err := Version(value); err != nil {
		return false
	}
	if r.Min != "" {
		c, err := Compare(value, r.Min)
		if err != nil || c < 0 {
			return false
		}
	}
	if r.MaxExclusive != "" {
		c, err := Compare(value, r.MaxExclusive)
		if err != nil || c >= 0 {
			return false
		}
	}
	return true
}
func (r VersionRange) Validate() error {
	for _, v := range []string{r.Min, r.MaxExclusive} {
		if v != "" {
			if _, err := Version(v); err != nil {
				return err
			}
		}
	}
	if r.Min != "" && r.MaxExclusive != "" {
		if c, _ := Compare(r.Min, r.MaxExclusive); c >= 0 {
			return fmt.Errorf("empty version range")
		}
	}
	return nil
}
func ValidHash(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == 32 && value == strings.ToLower(value)
}
func (m Manifest) Validate() error {
	if !ValidID(m.ID) || m.ID == "core" || m.Name == "" || len(m.Name) > 128 || m.API != APIVersion {
		return fmt.Errorf("invalid extension identity or API")
	}
	if _, err := Version(m.Version); err != nil {
		return err
	}
	if m.Kind != "frontend-sandbox" && m.Kind != "frontend-trusted" && m.Kind != "data" && m.Kind != "runtime" && m.Kind != "service" {
		return fmt.Errorf("unsupported extension kind %q", m.Kind)
	}
	if m.Target != "backend" && m.Target != "client" {
		return fmt.Errorf("invalid installation target")
	}
	if m.Execution != "" && m.Execution != "trusted-process" && m.Execution != "android-service" {
		return fmt.Errorf("unsupported execution mode %q", m.Execution)
	}
	if m.Execution != "" && m.Kind != "service" && (m.Kind != "runtime" || !SafePath(m.Entry) || m.Files[m.Entry] == "") {
		return fmt.Errorf("invalid runtime entry")
	}
	if m.Execution != "" && m.Kind != "service" {
		granted := false
		for _, p := range m.Permissions {
			granted = granted || p == "runtime.execute"
		}
		if !granted {
			return fmt.Errorf("runtime requires runtime.execute permission")
		}
	}
	if m.Execution == "android-service" {
		a := m.Android
		if a == nil || a.Format != "cph-host-v1" || a.MinSDK < 24 || (a.ABI != "arm64-v8a" && a.ABI != "x86_64") || a.Library != "lib/"+a.ABI+"/libcphlua.so" || a.Library != m.Entry || len(a.Files) != len(m.Files) {
			return fmt.Errorf("invalid Android runtime")
		}
		for name, hash := range m.Files {
			if a.Files[name] != hash {
				return fmt.Errorf("Android runtime hashes differ")
			}
		}
	}
	if m.Activation != "hot" && m.Activation != "restart" && m.Activation != "system-install" {
		return fmt.Errorf("invalid activation")
	}
	if err := m.Core.Validate(); err != nil {
		return err
	}
	if err := ValidateEnvironments(m.Environments); err != nil {
		return err
	}
	if err := m.validateBackend(); err != nil {
		return err
	}
	if m.Storage != nil {
		if m.Kind != "service" && m.Kind != "frontend-sandbox" && m.Kind != "data" {
			return fmt.Errorf("storage is only available to functional extensions")
		}
		if m.Target != "backend" || (!HasPermission(m.Permissions, "storage.read") && !HasPermission(m.Permissions, "storage.write")) {
			return fmt.Errorf("storage requires a backend installation and explicit storage permission")
		}
		if err := m.Storage.Validate(); err != nil {
			return err
		}
	}
	for id, r := range m.Dependencies {
		if !ValidID(id) || id == m.ID {
			return fmt.Errorf("invalid dependency %q", id)
		}
		if err := r.Validate(); err != nil {
			return err
		}
	}
	for _, values := range [][]string{m.Permissions, m.Capabilities} {
		seen := map[string]bool{}
		for _, v := range values {
			if !ValidID(v) || seen[v] {
				return fmt.Errorf("invalid or duplicate identifier %q", v)
			}
			seen[v] = true
		}
	}
	pages := map[string]bool{}
	for _, p := range m.Pages {
		if !ValidID(p.ID) || !SafePath(p.Entry) || pages[p.ID] || m.Files[p.Entry] == "" {
			return fmt.Errorf("invalid page %q", p.ID)
		}
		pages[p.ID] = true
	}
	ids := map[string]bool{}
	for _, c := range m.Contributions {
		if !ValidID(c.ID) || ids[c.ID] || !pages[c.Page] || (c.Location != "plugins.toolbar" && c.Location != "plugins.item.actions") || (c.When != "" && c.When != "editable") {
			return fmt.Errorf("invalid contribution %q", c.ID)
		}
		ids[c.ID] = true
	}
	for _, a := range m.Actions {
		if !ValidID(a.ID) || ids[a.ID] {
			return fmt.Errorf("invalid action %q", a.ID)
		}
		if a.Target != "" {
			if !ValidID(a.Target) || a.Input != nil || a.Output != nil || a.Permission != "" || a.Effect != "" || a.TimeoutMS != 0 {
				return fmt.Errorf("invalid action alias %q", a.ID)
			}
		} else {
			if m.Backend == nil || !ValidID(a.Permission) || !HasPermission(m.Permissions, a.Permission) || a.Input == nil || a.Output == nil || a.TimeoutMS < 1 || a.TimeoutMS > 300000 || (a.Effect != "read" && a.Effect != "write" && a.Effect != "execute") {
				return fmt.Errorf("invalid backend action %q", a.ID)
			}
			if err := a.Input.Check(); err != nil {
				return err
			}
			if err := a.Output.Check(); err != nil {
				return err
			}
		}
		ids[a.ID] = true
	}
	if err := ValidateSettings(m.Settings); err != nil {
		return err
	}
	for path, hash := range m.Files {
		if !SafePath(path) || !ValidHash(hash) || strings.EqualFold(path, "manifest.json") || strings.EqualFold(path, "signature.json") || strings.EqualFold(path, "package.cphext") || strings.EqualFold(path, "package.cphhost") {
			return fmt.Errorf("invalid file %q", path)
		}
	}
	return nil
}
