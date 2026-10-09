package extension

import (
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

type TrustEntry struct {
	Code     string        `json:"code"`
	Config   spec.TrustKey `json:"config"`
	ReadOnly bool          `json:"read_only"`
}

func copyTrust(source spec.TrustStore) spec.TrustStore {
	result := spec.TrustStore{}
	for id, key := range source {
		key.IDs = append([]string{}, key.IDs...)
		key.Permissions = append([]string{}, key.Permissions...)
		result[id] = key
	}
	return result
}

func (m *Manager) trustSnapshot() spec.TrustStore {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return copyTrust(m.trust)
}

func validateTrust(code string, key spec.TrustKey) error {
	if !spec.ValidID(code) || strings.TrimSpace(key.Publisher) == "" || len(key.Publisher) > 200 {
		return fmt.Errorf("valid signing key code and publisher are required")
	}
	if (key.PublicKey == "") == (key.Certificate == "") {
		return fmt.Errorf("provide exactly one public_key or certificate")
	}
	if key.PublicKey != "" {
		public, err := base64.StdEncoding.DecodeString(key.PublicKey)
		if err != nil || len(public) != ed25519.PublicKeySize {
			return fmt.Errorf("public_key must be a Base64 Ed25519 public key")
		}
	} else {
		der, err := base64.StdEncoding.DecodeString(key.Certificate)
		if err != nil {
			return fmt.Errorf("certificate must contain Base64 DER")
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return fmt.Errorf("invalid signing certificate")
		}
		public, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok || public.N.BitLen() < 2048 {
			return fmt.Errorf("signing certificate requires an RSA key of at least 2048 bits")
		}
	}
	if len(key.IDs) == 0 || len(key.IDs) > 256 || len(key.Permissions) > 64 {
		return fmt.Errorf("provide 1..256 package IDs and at most 64 permissions")
	}
	for _, values := range [][]string{key.IDs, key.Permissions} {
		seen := map[string]bool{}
		for _, value := range values {
			if !spec.ValidID(value) || seen[value] {
				return fmt.Errorf("invalid or duplicate package ID or permission: %s", value)
			}
			seen[value] = true
		}
	}
	return nil
}

func (m *Manager) mergedTrust(managed spec.TrustStore) (spec.TrustStore, error) {
	if len(managed) > 128 {
		return nil, fmt.Errorf("too many signing identities")
	}
	merged := copyTrust(m.fixedTrust)
	for code, key := range managed {
		if _, exists := merged[code]; exists {
			return nil, fmt.Errorf("signing identity %s is managed by the distribution or trust file", code)
		}
		if err := validateTrust(code, key); err != nil {
			return nil, err
		}
		merged[code] = key
	}
	return merged, nil
}

// ConfigureManagedTrust 在加载扩展前接入数据库，发行身份始终保持只读。
func (m *Manager) ConfigureManagedTrust(managed spec.TrustStore, save func(spec.TrustStore) error) error {
	m.op.Lock()
	defer m.op.Unlock()
	merged, err := m.mergedTrust(managed)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.managedTrust, m.trust, m.saveTrust = copyTrust(managed), copyTrust(merged), save
	m.mu.Unlock()
	return nil
}

func (m *Manager) TrustEntries() []TrustEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entries := []TrustEntry{}
	for code, key := range copyTrust(m.trust) {
		_, fixed := m.fixedTrust[code]
		entries = append(entries, TrustEntry{Code: code, Config: key, ReadOnly: fixed})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Code < entries[j].Code })
	return entries
}

// UpdateTrust 与安装串行；不得撤销正在使用的身份，持久化成功后才生效。
func (m *Manager) UpdateTrust(code string, config *spec.TrustKey) error {
	m.op.Lock()
	defer m.op.Unlock()
	if _, fixed := m.fixedTrust[code]; fixed {
		return fmt.Errorf("this signing identity is read-only")
	}
	if m.saveTrust == nil {
		return fmt.Errorf("managed signing identities are unavailable")
	}
	next := copyTrust(m.managedTrust)
	if config == nil {
		if _, exists := next[code]; !exists {
			return fmt.Errorf("signing identity does not exist")
		}
		delete(next, code)
	} else {
		next[code] = *config
	}
	merged, err := m.mergedTrust(next)
	if err != nil {
		return err
	}
	for _, state := range m.List() {
		if state.Signer == code && state.Enabled {
			if _, err := Verify(m.Archive(state), merged); err != nil {
				return fmt.Errorf("disable extension %s before revoking its signing identity: %w", state.Manifest.ID, err)
			}
		}
	}
	if err := m.saveTrust(copyTrust(next)); err != nil {
		return err
	}
	m.mu.Lock()
	m.managedTrust, m.trust = copyTrust(next), copyTrust(merged)
	m.mu.Unlock()
	return nil
}
