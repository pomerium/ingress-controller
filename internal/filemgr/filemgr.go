// Package filemgr contains a manager for files based on byte slices.
package filemgr

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/martinlindhe/base36"

	"github.com/pomerium/pomerium/pkg/cryptutil"
)

// A Manager manages temporary files created from data.
type Manager struct {
	cacheDir string
}

// New creates a new Manager.
func New(cacheDir string) *Manager {
	return &Manager{
		cacheDir: cacheDir,
	}
}

// CreateFile creates a new file based on the passed in data.
func (mgr *Manager) CreateFile(fileName string, data []byte) (filePath string, err error) {
	h := base36.EncodeBytes(cryptutil.Hash("filemgr", data))
	ext := filepath.Ext(fileName)
	fileName = fmt.Sprintf("%s-%x%s", fileName[:len(fileName)-len(ext)], h, ext)
	filePath = filepath.Join(mgr.cacheDir, fileName)

	if err := os.MkdirAll(mgr.cacheDir, 0o700); err != nil {
		return filePath, fmt.Errorf("filemgr: error creating cache directory: %w", err)
	}

	_, err = os.Stat(filePath)
	if err == nil {
		return filePath, nil
	}

	err = os.WriteFile(filePath, data, 0o600)
	if err != nil {
		_ = os.Remove(filePath)
		return filePath, fmt.Errorf("filemgr: error writing file: %w", err)
	}

	err = os.Chmod(filePath, 0o400)
	if err != nil {
		_ = os.Remove(filePath)
		return filePath, fmt.Errorf("filemgr: error chmoding file: %w", err)
	}

	return filePath, nil
}

// DeleteFiles deletes all the files managed by the file manager.
func (mgr *Manager) DeleteFiles() error {
	if _, err := os.Stat(mgr.cacheDir); os.IsNotExist(err) {
		return nil
	}

	return filepath.Walk(mgr.cacheDir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			return os.Remove(p)
		}
		return nil
	})
}
