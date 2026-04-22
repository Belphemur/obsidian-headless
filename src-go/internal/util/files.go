package util

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/text/unicode/norm"
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
	fullPath, err := SafeJoin(root, record.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(fullPath, content, 0o644); err != nil {
		return err
	}
	mtime := time.UnixMilli(record.MTime)
	return os.Chtimes(fullPath, mtime, mtime)
}

func SafeJoin(root, relative string) (string, error) {
	cleaned := path.Clean(strings.TrimSpace(relative))
	if cleaned == "." || cleaned == "" || path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("invalid relative path %q", relative)
	}
	baseRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(baseRoot, filepath.FromSlash(cleaned))
	resolved, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	separator := string(os.PathSeparator)
	if resolved != baseRoot && !strings.HasPrefix(resolved, baseRoot+separator) {
		return "", fmt.Errorf("path %q escapes vault root", relative)
	}
	return resolved, nil
}

func DerivePasswordHash(password, salt string) (string, error) {
	normalizedPassword := norm.NFKC.String(password)
	normalizedSalt := norm.NFKC.String(salt)
	key, err := scrypt.Key([]byte(normalizedPassword), []byte(normalizedSalt), 1<<15, 8, 1, 32)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}
