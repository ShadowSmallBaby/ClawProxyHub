package extension

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func TestAndroidHostUsesApplicationCertificateTrust(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certHash := sha256.Sum256(der)
	id := "android-app:" + hex.EncodeToString(certHash[:])
	encoded := base64.StdEncoding.EncodeToString(der)
	trust := spec.TrustStore{id: {Certificate: encoded, IDs: []string{"lua-runtime"}, Permissions: []string{"runtime.execute"}, Native: true}}
	library := []byte("signed Android library")
	sum := sha256.Sum256(library)
	path := "lib/arm64-v8a/libcphlua.so"
	files := map[string]string{path: hex.EncodeToString(sum[:])}
	manifest := spec.Manifest{
		ID: "lua-runtime", Name: "luahost", Version: "0.2.0", API: spec.APIVersion,
		Kind: "runtime", Target: "backend", Activation: "hot", Execution: "android-service",
		Core: spec.VersionRange{Min: "1.5.2"}, Permissions: []string{"runtime.execute"}, Capabilities: []string{"lua"},
		Platforms: []string{"android/arm64"}, Entry: path, Files: files,
		Android: &spec.AndroidRuntime{Format: "cph-host-v1", ABI: "arm64-v8a", Library: path, MinSDK: 24, Files: files},
	}
	raw, _ := json.Marshal(manifest)
	digest := sha256.Sum256(raw)
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	signed, _ := json.Marshal(spec.Signature{Algorithm: "SHA256withRSA", Certificate: encoded, Value: base64.StdEncoding.EncodeToString(signature)})
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for name, data := range map[string][]byte{"manifest.json": raw, "signature.json": signed, path: library} {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	v, err := VerifyBytes(output.Bytes(), trust)
	if err != nil {
		t.Fatal(err)
	}
	if v.Manifest.ID != "lua-runtime" || v.Manifest.Execution != "android-service" || v.Signature.KeyID != id {
		t.Fatal("Android host identity or certificate key differs")
	}
	if _, err = VerifyBytes(output.Bytes(), spec.TrustStore{}); err == nil {
		t.Fatal("package-provided certificate established trust")
	}
	limited := trust[id]
	limited.Native = false
	if _, err = VerifyBytes(output.Bytes(), spec.TrustStore{id: limited}); err == nil {
		t.Fatal("certificate bypassed native permission")
	}
	limited = trust[id]
	limited.IDs = []string{"editor"}
	if _, err = VerifyBytes(output.Bytes(), spec.TrustStore{id: limited}); err == nil {
		t.Fatal("certificate bypassed identity scope")
	}
}
