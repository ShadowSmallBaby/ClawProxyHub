package extension

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

// StageUpload 只接收已验证包，动作只接受不可猜的暂存票据，不暴露任意服务器路径。
func (m *Manager) StageUpload(filename string) (string, error) {
	v, err := m.Inspect(filename)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(m.dir, "uploads")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !e.IsDir() && time.Since(info.ModTime()) > time.Hour {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	nonce := make([]byte, 32)
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	ticket := hex.EncodeToString(nonce)
	f, err := os.OpenFile(filepath.Join(dir, ticket+".cphext"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, err = f.Write(v.Archive)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		m.DiscardUpload(ticket)
		return "", err
	}
	return ticket, nil
}
func (m *Manager) DiscardUpload(ticket string) {
	if spec.ValidHash(ticket) {
		os.Remove(filepath.Join(m.dir, "uploads", ticket+".cphext"))
	}
}
func (m *Manager) InstallUpload(ctx context.Context, ticket string, grants []string) (State, error) {
	if !spec.ValidHash(ticket) {
		return State{}, fmt.Errorf("invalid extension upload ticket")
	}
	defer m.DiscardUpload(ticket)
	return m.Install(ctx, filepath.Join(m.dir, "uploads", ticket+".cphext"), grants)
}
