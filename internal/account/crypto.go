// crypto.go — 凭据加密存储：AES-256-GCM。
// 密钥来源：CPH_SECRET_KEY（hex）或自动生成并落盘 <data>/secret.key。
// 加密数据以 0x01 前缀标记；解密失败按明文返回（平滑兼容存量数据）。
package account

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const encPrefix = byte(0x01)

// KeyLookupHash 返回 apikey 明文的确定性查找哈希（sha256 hex，64 字符）。
// 高熵随机 key 无需加盐；用作 keys.key_lookup 索引列，鉴权按等值 O(1) 命中，
// 免去 AES-GCM 密文（nonce 随机不可等值查）导致的全表解密扫描。
// 与存量 sha256 hex 密钥（KeyCipher 即本值）天然一致，故可直接回填。
func KeyLookupHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

var (
	keyOnce  sync.Once
	aead     cipher.AEAD
	keyError error
)

// loadKey 初始化加密器（进程内一次）。
func loadKey(dataDir string) (cipher.AEAD, error) {
	keyOnce.Do(func() {
		key := loadOrCreateKey(dataDir)
		block, err := aes.NewCipher(key)
		if err != nil {
			keyError = err
			return
		}
		aead, keyError = cipher.NewGCM(block)
	})
	return aead, keyError
}

// loadOrCreateKey 取密钥：env(hex) > data/secret.key（自动生成）。
func loadOrCreateKey(dataDir string) []byte {
	if env := os.Getenv("CPH_SECRET_KEY"); env != "" {
		if key, err := hex.DecodeString(env); err == nil && len(key) == 32 {
			return key
		}
		// 非 hex 的按原始字节取（凑满 32 字节即可用）
		if len(env) == 32 {
			return []byte(env)
		}
	}
	keyPath := filepath.Join(dataDir, "secret.key")
	if b, err := os.ReadFile(keyPath); err == nil && len(b) == 32 {
		return b
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		panic("generate secret key: " + err.Error())
	}
	_ = os.MkdirAll(dataDir, 0o700)
	_ = os.WriteFile(keyPath, key, 0o600)
	return key
}

// EncryptCredential 加密凭据 blob；未初始化加密器时原样返回。
func EncryptCredential(dataDir string, blob []byte) []byte {
	gcm, err := loadKey(dataDir)
	if err != nil || len(blob) == 0 {
		return blob
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return blob
	}
	sealed := gcm.Seal(nil, nonce, blob, nil)
	return append([]byte{encPrefix}, append(nonce, sealed...)...)
}

// DecryptCredential 解密凭据 blob；非加密格式（无 0x01 前缀）按明文返回。
func DecryptCredential(dataDir string, data []byte) []byte {
	if len(data) == 0 || data[0] != encPrefix {
		return data
	}
	gcm, err := loadKey(dataDir)
	if err != nil {
		return data
	}
	nonceSize := gcm.NonceSize()
	if len(data) < 1+nonceSize+gcm.Overhead() {
		return data
	}
	nonce, sealed := data[1:1+nonceSize], data[1+nonceSize:]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return data // 解不开（密钥不匹配的存量）按原样返回，交由插件报凭据错误
	}
	return plain
}
