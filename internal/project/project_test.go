package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanProjectDirs(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "alpha"), 0755)
	os.Mkdir(filepath.Join(root, "beta"), 0755)
	os.WriteFile(filepath.Join(root, "file.txt"), []byte("not a dir"), 0644)

	dirs := ScanProjectDirs(root)
	if len(dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d", len(dirs))
	}
	names := map[string]bool{}
	for _, d := range dirs {
		names[d.Name] = true
	}
	if !names["alpha"] || !names["beta"] {
		t.Errorf("expected alpha and beta, got %v", names)
	}
}

func TestScanProjectDirs_Empty(t *testing.T) {
	root := t.TempDir()
	dirs := ScanProjectDirs(root)
	if len(dirs) != 0 {
		t.Fatalf("expected 0 dirs, got %d", len(dirs))
	}
}

func TestScanProjectDirs_NonexistentDir(t *testing.T) {
	dirs := ScanProjectDirs("/nonexistent/path")
	if dirs != nil {
		t.Fatalf("expected nil, got %v", dirs)
	}
}
