package watch

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetInode_Unix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("inode not available on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	ino := getInode(info)
	if ino == 0 {
		t.Fatal("expected non-zero inode on Unix")
	}
}

func TestGetInode_Nil(t *testing.T) {
	if ino := getInode(nil); ino != 0 {
		t.Fatalf("expected 0 for nil FileInfo, got %d", ino)
	}
}

func TestGetInode_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("test only valid on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	ino := getInode(info)
	if ino != 0 {
		t.Fatalf("expected 0 inode on Windows, got %d", ino)
	}
}

func TestScanner_HasChanged_InodeDetection(t *testing.T) {
	s := NewScanner()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	// Create initial file
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	changed, eventType := s.HasChanged(path)
	if !changed {
		t.Fatal("expected change for new file")
	}
	if eventType != EventCreate {
		t.Fatalf("expected EventCreate, got %s", eventType)
	}

	// Same file, no change
	changed, _ = s.HasChanged(path)
	if changed {
		t.Fatal("expected no change for identical file")
	}

	// Modify the file
	if err := os.WriteFile(path, []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}
	changed, eventType = s.HasChanged(path)
	if !changed {
		t.Fatal("expected change for modified file")
	}
	if eventType != EventWrite {
		t.Fatalf("expected EventWrite, got %s", eventType)
	}
}

func TestScanner_GetInode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("inode not available on Windows")
	}
	s := NewScanner()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	s.Update(path)

	ino, ok := s.GetInode(path)
	if !ok {
		t.Fatal("expected inode to be found")
	}
	if ino == 0 {
		t.Fatal("expected non-zero inode on Unix")
	}

	// Non-existent path
	_, ok = s.GetInode("/nonexistent/path")
	if ok {
		t.Fatal("expected false for non-existent path")
	}
}

func TestScanner_GetInode_Missing(t *testing.T) {
	s := NewScanner()
	_, ok := s.GetInode("/nonexistent")
	if ok {
		t.Fatal("expected false for missing path")
	}
}

func TestScanner_UpdateInfo(t *testing.T) {
	s := NewScanner()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	s.UpdateInfo(path, info)

	changed, _ := s.HasChanged(path)
	if changed {
		t.Fatal("expected no change after UpdateInfo")
	}
}

func TestScanner_Remove(t *testing.T) {
	s := NewScanner()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	s.Update(path)

	_, ok := s.GetInode(path)
	if !ok {
		t.Fatal("expected inode to exist before remove")
	}

	s.Remove(path)
	_, ok = s.GetInode(path)
	if ok {
		t.Fatal("expected inode to not exist after remove")
	}
}
