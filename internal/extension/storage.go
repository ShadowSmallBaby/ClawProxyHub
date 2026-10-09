package extension

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func (m *Manager) store(v *Verified) (int64, error) {
	base := filepath.Join(m.directory(v.Manifest), "versions", v.Manifest.ID)
	if err := os.MkdirAll(m.dir, 0700); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return 0, err
	}
	stage, err := os.MkdirTemp(base, ".stage-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(stage)
	var size int64
	for name, data := range v.Files {
		path := filepath.Join(stage, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return 0, err
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			return 0, err
		}
		size += int64(len(data))
	}
	if err = os.WriteFile(filepath.Join(stage, archiveName(v.Manifest)), v.Archive, 0600); err != nil {
		return 0, err
	}
	destination := filepath.Join(base, v.SHA256)
	if _, err = os.Stat(destination); errors.Is(err, fs.ErrNotExist) {
		err = os.Rename(stage, destination)
	}
	return size, err
}

// migrateRuntime 从已验签的归档重建运行时目录，保留原安装的启停状态。
func (m *Manager) migrateRuntime(s State) error {
	if s.Manifest.Kind != "runtime" {
		return nil
	}
	if _, err := os.Stat(m.Archive(s)); !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	previous := filepath.Join(m.dir, "versions", s.Manifest.ID, s.Hash)
	archive := filepath.Join(previous, "package.cphext")
	if _, err := os.Stat(archive); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	v, err := m.Inspect(archive)
	if err != nil {
		return err
	}
	if v.SHA256 != s.Hash || v.Manifest.ID != s.Manifest.ID || v.Manifest.Kind != "runtime" {
		return fmt.Errorf("stored runtime identity mismatch")
	}
	if _, err = m.store(v); err != nil {
		return err
	}
	return os.RemoveAll(previous)
}
