// Package atomicfile replaces files so that readers see either the old or the new contents.
package atomicfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Write writes data to a temporary file next to path, syncs it, and renames it over path. An existing
// file keeps its mode, and a new file gets perm. When path is a symbolic link, the file it points to is
// replaced. Hard links to path are not updated. On failure, path is left unchanged.
func Write(path string, data []byte, perm fs.FileMode) error {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	err = f.Chmod(perm)
	if err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmpPath, path)
	}
	if err != nil {
		_ = os.Remove(tmpPath) // Best effort. The original file is untouched.
	}
	return err
}
