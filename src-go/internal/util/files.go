package util

import (
"crypto/rand"
"crypto/sha256"
"encoding/hex"
"io"
"os"
"path/filepath"
"sort"
"strings"
"time"

"github.com/Belphemur/obsidian-headless/src-go/internal/model"
)

func RandomHex(size int) (string, error) {
buf := make([]byte, size)
if _, err := rand.Read(buf); err != nil {
return "", err
}
return hex.EncodeToString(buf), nil
}

func HashBytes(data []byte) string {
sum := sha256.Sum256(data)
return hex.EncodeToString(sum[:])
}

func HashReader(reader io.Reader) (string, error) {
h := sha256.New()
if _, err := io.Copy(h, reader); err != nil {
return "", err
}
return hex.EncodeToString(h.Sum(nil)), nil
}

func ScanVault(root, configDir string, ignored []string) (map[string]model.FileRecord, error) {
root, err := filepath.Abs(root)
if err != nil {
return nil, err
}
ignoredSet := map[string]struct{}{}
for _, folder := range ignored {
ignoredSet[filepath.Clean(strings.Trim(folder, "/"))] = struct{}{}
}
files := map[string]model.FileRecord{}
err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
if walkErr != nil {
return walkErr
}
if path == root {
return nil
}
rel, err := filepath.Rel(root, path)
if err != nil {
return err
}
rel = filepath.ToSlash(rel)
if rel == configDir || strings.HasPrefix(rel, configDir+"/") || strings.HasPrefix(rel, ".git/") || rel == ".git" {
if d.IsDir() {
return filepath.SkipDir
}
return nil
}
for folder := range ignoredSet {
if rel == folder || strings.HasPrefix(rel, folder+"/") {
if d.IsDir() {
return filepath.SkipDir
}
return nil
}
}
if d.IsDir() {
return nil
}
info, err := d.Info()
if err != nil {
return err
}
data, err := os.ReadFile(path)
if err != nil {
return err
}
mtime := info.ModTime().UnixMilli()
files[rel] = model.FileRecord{
Path:   rel,
Size:   info.Size(),
Hash:   HashBytes(data),
CTime:  mtime,
MTime:  mtime,
Folder: false,
}
return nil
})
if err != nil {
return nil, err
}
return files, nil
}

func SortedPaths(records map[string]model.FileRecord) []string {
paths := make([]string, 0, len(records))
for path := range records {
paths = append(paths, path)
}
sort.Strings(paths)
return paths
}

func WriteFileWithTimes(root string, record model.FileRecord, content []byte) error {
fullPath := filepath.Join(root, filepath.FromSlash(record.Path))
if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
return err
}
if err := os.WriteFile(fullPath, content, 0o644); err != nil {
return err
}
mtime := time.UnixMilli(record.MTime)
return os.Chtimes(fullPath, mtime, mtime)
}
