package filemgr

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestManager(t *testing.T) {
	dir := filepath.Join(os.TempDir(), uuid.New().String())
	defer os.RemoveAll(dir)

	mgr := New(dir)
	fp1, err := mgr.CreateFile("hello.txt", []byte("HELLO WORLD"))
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "hello-32474a4f41355432494e594e58334e4b4b4453483555314e4842584544424139375148533858303543434f4e56524b43374a.txt"), fp1)

	fp2, err := mgr.CreateFile("empty", nil)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "empty-314a323947555a5055304f45304944514c4f5242384244493339453533505551393131494e484f545353425a443759435453"), fp2)

	assert.Equal(t, 2, countFiles(dir))
	assert.NoError(t, mgr.DeleteFiles())
	assert.Equal(t, 0, countFiles(dir))
}

func countFiles(dir string) int {
	fileCount := 0
	filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if !info.IsDir() {
			fileCount++
		}
		return nil
	})
	return fileCount
}
