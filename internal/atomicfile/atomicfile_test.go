package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")
	if err := Write(path, []byte("a"), 0o640); err != nil {
		t.Fatal(err)
	}
	assertFile(t, path, "a", 0o640)

	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertFile(t, path, "b", 0o600)

	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink("new.txt", link); err != nil {
		t.Fatal(err)
	}
	if err := Write(link, []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertFile(t, path, "c", 0o600)
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symbolic link replaced: %v, %v", info, err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("found %d directory entries, expected 2 with no temporary files", len(entries))
	}

	if err := Write(filepath.Join(dir, "missing", "x.txt"), []byte("d"), 0o644); err == nil {
		t.Error("expected error for a missing directory")
	}
}

func assertFile(t *testing.T, path, data string, perm os.FileMode) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != data {
		t.Errorf("%s = %q, expected %q", path, b, data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != perm {
		t.Errorf("%s mode = %o, expected %o", path, info.Mode().Perm(), perm)
	}
}
