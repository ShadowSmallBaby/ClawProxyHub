// crypto.go — 主密钥可靠落盘后才启用加密，损坏的加密记录必须报错。
package account

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const encPrefix = byte(0x01)

func KeyLookupHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

var keyMu sync.Mutex
var ciphers = map[string]cipher.AEAD{}

func InitCrypto(dataDir string) error { _, err := loadKey(dataDir); return err }

// BackupKey 导出当前实际密钥，环境变量密钥与文件密钥遵循同一优先级。
func BackupKey(dataDir string) ([]byte, error) {
	keyMu.Lock()
	defer keyMu.Unlock()
	return loadOrCreateKey(dataDir)
}

func loadKey(dataDir string) (cipher.AEAD, error) {
	keyMu.Lock()
	defer keyMu.Unlock()
	path, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	cacheKey := path + "\x00" + os.Getenv("CPH_SECRET_KEY")
	if c := ciphers[cacheKey]; c != nil {
		return c, nil
	}
	key, err := loadOrCreateKey(dataDir)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err == nil {
		ciphers[cacheKey] = gcm
	}
	return gcm, err
}

func loadOrCreateKey(dataDir string) ([]byte, error) {
	if env := os.Getenv("CPH_SECRET_KEY"); env != "" {
		if key, err := hex.DecodeString(env); err == nil && len(key) == 32 {
			return key, nil
		}
		if len(env) == 32 {
			return []byte(env), nil
		}
		return nil, errors.New("CPH_SECRET_KEY must contain 32 bytes or 64 hex characters")
	}
	path := filepath.Join(dataDir, "secret.key")
	if key, err := os.ReadFile(path); err == nil {
		if len(key) != 32 {
			return nil, errors.New("invalid secret.key length")
		}
		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	rand.Read(key)
	tmp, err := os.CreateTemp(dataDir, ".secret-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return nil, err
	}
	if _, err := tmp.Write(key); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	// 硬链接以排他方式发布完整文件，避免并发首启互相覆盖。
	if err := os.Link(tmp.Name(), path); err != nil {
		if errors.Is(err, os.ErrExist) {
			existing, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil, readErr
			}
			if len(existing) != 32 {
				return nil, errors.New("invalid secret.key length")
			}
			return existing, nil
		}
		return nil, fmt.Errorf("persist secret.key: %w", err)
	}
	return key, nil
}

func EncryptCredential(dataDir string, blob []byte) ([]byte, error) {
	if len(blob) == 0 {
		return nil, nil
	}
	gcm, err := loadKey(dataDir)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)
	return gcm.Seal(append([]byte{encPrefix}, nonce...), nonce, blob, nil), nil
}

func DecryptCredential(dataDir string, data []byte) ([]byte, error) {
	if len(data) == 0 || data[0] != encPrefix {
		return data, nil
	}
	gcm, err := loadKey(dataDir)
	if err != nil {
		return nil, err
	}
	n := gcm.NonceSize()
	if len(data) < 1+n+gcm.Overhead() {
		return nil, errors.New("truncated encrypted credential")
	}
	return gcm.Open(nil, data[1:1+n], data[1+n:], nil)
}
