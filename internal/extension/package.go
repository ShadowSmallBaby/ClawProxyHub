// Package extension 校验签名扩展包并管理版本、依赖和激活状态。
package extension

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

const MaxPackageBytes = 128 << 20

type Verified struct {
	Manifest  spec.Manifest
	Signature spec.Signature
	Publisher string
	SHA256    string
	Files     map[string][]byte
	Archive   []byte
}

// Verify 使用宿主信任根验证原始 manifest 与全部文件，包内声明不能增加信任。
func Verify(filename string, trust spec.TrustStore) (*Verified, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxPackageBytes+1))
	if err != nil {
		return nil, err
	}
	return VerifyBytes(data, trust)
}
func VerifyBytes(data []byte, trust spec.TrustStore) (*Verified, error) {
	if len(data) > MaxPackageBytes {
		return nil, fmt.Errorf("extension package too large")
	}
	h := sha256.Sum256(data)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	seen := map[string]bool{}
	var total uint64
	for _, entry := range zr.File {
		name := strings.TrimSuffix(entry.Name, "/")
		folded := strings.ToLower(name)
		if !spec.SafePath(name) || entry.Mode()&os.ModeSymlink != 0 || seen[folded] {
			return nil, fmt.Errorf("unsafe or duplicate extension path %q", entry.Name)
		}
		seen[folded] = true
		if entry.FileInfo().IsDir() {
			continue
		}
		if len(files) >= 4096 || entry.UncompressedSize64 > MaxPackageBytes || total > MaxPackageBytes-entry.UncompressedSize64 {
			return nil, fmt.Errorf("extension exceeds extraction limit")
		}
		total += entry.UncompressedSize64
		r, err := entry.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(r, MaxPackageBytes+1))
		r.Close()
		if err != nil || len(data) > MaxPackageBytes {
			return nil, fmt.Errorf("read extension entry %q", name)
		}
		files[name] = data
	}
	v := &Verified{Files: files, SHA256: hex.EncodeToString(h[:]), Archive: data}
	if len(files["manifest.json"]) == 0 || len(files["manifest.json"]) > 1<<20 {
		return nil, fmt.Errorf("missing or oversized extension manifest")
	}
	if err := json.Unmarshal(files["manifest.json"], &v.Manifest); err != nil {
		return nil, fmt.Errorf("invalid extension manifest: %w", err)
	}
	if err := json.Unmarshal(files["signature.json"], &v.Signature); err != nil {
		return nil, fmt.Errorf("invalid extension signature metadata: %w", err)
	}
	if v.Signature.KeyID == "" && v.Signature.Certificate != "" {
		certificate, err := base64.StdEncoding.DecodeString(v.Signature.Certificate)
		if err != nil {
			return nil, fmt.Errorf("invalid signing certificate")
		}
		digest := sha256.Sum256(certificate)
		v.Signature.KeyID = "android-app:" + hex.EncodeToString(digest[:])
	}
	if err = v.Manifest.Validate(); err != nil {
		return nil, err
	}
	key, ok := trust[v.Signature.KeyID]
	if !ok {
		return nil, fmt.Errorf("untrusted signing key %q", v.Signature.KeyID)
	}
	if err := verifySignature(files["manifest.json"], v.Signature, key); err != nil {
		return nil, err
	}
	allowed := false
	for _, id := range key.IDs {
		if id == v.Manifest.ID {
			allowed = true
		}
	}
	if !allowed {
		return nil, fmt.Errorf("signer is not authorized for %q", v.Manifest.ID)
	}
	for _, permission := range v.Manifest.Permissions {
		if !contains(key.Permissions, permission) {
			return nil, fmt.Errorf("signer cannot grant %q", permission)
		}
	}
	if (v.Manifest.Kind == "runtime" || v.Manifest.Kind == "service") && !key.Native {
		return nil, fmt.Errorf("native execution is not trusted")
	}
	if len(files) != len(v.Manifest.Files)+2 {
		return nil, fmt.Errorf("unsigned extension files")
	}
	for name, want := range v.Manifest.Files {
		data, ok := files[name]
		sum := sha256.Sum256(data)
		if !ok || hex.EncodeToString(sum[:]) != want {
			return nil, fmt.Errorf("extension hash mismatch: %s", name)
		}
	}
	v.Publisher = key.Publisher
	return v, nil
}

func verifySignature(raw []byte, signed spec.Signature, key spec.TrustKey) error {
	sig, err := base64.StdEncoding.DecodeString(signed.Value)
	if err != nil {
		return fmt.Errorf("invalid extension signature")
	}
	if key.Certificate != "" {
		der, err := base64.StdEncoding.DecodeString(key.Certificate)
		provided, decodeErr := base64.StdEncoding.DecodeString(signed.Certificate)
		if err != nil || decodeErr != nil || signed.Algorithm != "SHA256withRSA" || !bytes.Equal(der, provided) {
			return fmt.Errorf("signing certificate differs from host trust")
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return fmt.Errorf("invalid trusted certificate")
		}
		pub, ok := cert.PublicKey.(*rsa.PublicKey)
		digest := sha256.Sum256(raw)
		if !ok || pub.N.BitLen() < 2048 || rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig) != nil {
			return fmt.Errorf("invalid extension signature")
		}
		return nil
	}
	pub, err := base64.StdEncoding.DecodeString(key.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid trusted public key")
	}
	if (signed.Algorithm != "" && signed.Algorithm != "Ed25519") || signed.Certificate != "" || !ed25519.Verify(pub, raw, sig) {
		return fmt.Errorf("invalid extension signature")
	}
	return nil
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
