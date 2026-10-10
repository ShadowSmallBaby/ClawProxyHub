// cphext 为独立扩展生成可验证签名包，不内置发布私钥。
package main

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	extension "github.com/ShadowSmallBaby/ClawProxyHub/internal/extension"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func writeNew(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: cphext keygen|pack|verify [flags]")
	}
	flags := flag.NewFlagSet(os.Args[1], flag.ContinueOnError)
	keyFile := flags.String("key", "", "private key file (base64)")
	keyID := flags.String("key-id", "", "trusted signing key ID")
	dir := flags.String("dir", "", "extension directory")
	out := flags.String("out", "", "output file")
	trustFile := flags.String("trust", "", "host trust JSON")
	pkg := flags.String("package", "", "package to verify")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	switch os.Args[1] {
	case "keygen":
		if *keyFile == "" {
			return fmt.Errorf("--key is required")
		}
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		if err = writeNew(*keyFile, []byte(base64.StdEncoding.EncodeToString(priv))); err != nil {
			return err
		}
		fmt.Println(base64.StdEncoding.EncodeToString(pub))
		return nil
	case "verify":
		data, err := os.ReadFile(*trustFile)
		if err != nil {
			return err
		}
		var trust spec.TrustStore
		if err = json.Unmarshal(data, &trust); err != nil {
			return err
		}
		v, err := extension.Verify(*pkg, trust)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"manifest": v.Manifest, "sha256": v.SHA256, "publisher": v.Publisher})
	case "pack":
		if *keyID == "" || *dir == "" || *out == "" {
			return fmt.Errorf("--key-id, --dir and --out are required")
		}
		encoded, err := os.ReadFile(*keyFile)
		if err != nil {
			return err
		}
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
		if err != nil || len(key) != ed25519.PrivateKeySize {
			return fmt.Errorf("invalid signing key")
		}
		raw, err := os.ReadFile(filepath.Join(*dir, "manifest.json"))
		if err != nil {
			return err
		}
		var manifest spec.Manifest
		if err = json.Unmarshal(raw, &manifest); err != nil {
			return err
		}
		manifest.Files = map[string]string{}
		files := map[string][]byte{}
		var total int
		err = filepath.WalkDir(*dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("symbolic links are not permitted")
			}
			name, err := filepath.Rel(*dir, path)
			if err != nil {
				return err
			}
			name = filepath.ToSlash(name)
			if name == "manifest.json" {
				return nil
			}
			if !spec.SafePath(name) {
				return fmt.Errorf("unsafe package path")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			total += len(data)
			if total > extension.MaxPackageBytes {
				return fmt.Errorf("package too large")
			}
			sum := sha256.Sum256(data)
			files[name] = data
			manifest.Files[name] = hex.EncodeToString(sum[:])
			return nil
		})
		if err != nil {
			return err
		}
		if err = manifest.Validate(); err != nil {
			return err
		}
		files["manifest.json"], err = json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return err
		}
		files["signature.json"], err = json.Marshal(spec.Signature{KeyID: *keyID, Value: base64.StdEncoding.EncodeToString(ed25519.Sign(key, files["manifest.json"]))})
		if err != nil {
			return err
		}
		f, err := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		success := false
		defer func() {
			f.Close()
			if !success {
				os.Remove(*out)
			}
		}()
		w := zip.NewWriter(f)
		names := []string{}
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			entry, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
			if err != nil {
				return err
			}
			if _, err = entry.Write(files[name]); err != nil {
				return err
			}
		}
		if err = w.Close(); err != nil {
			return err
		}
		if err = f.Sync(); err != nil {
			return err
		}
		success = true
		fmt.Println(*out)
		return nil
	default:
		return fmt.Errorf("unknown command %s", os.Args[1])
	}
}
