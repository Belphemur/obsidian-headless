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
	"golang.org/x/crypto/hkdf"
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
		if rel == ".git" || strings.HasPrefix(rel, ".git/") {
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
		// Skip hidden files (match TypeScript: paths starting with ".")
		if strings.HasPrefix(filepath.Base(rel), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			// Skip symlinks to avoid following pointers outside the vault.
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		hash, hashErr := HashReader(file)
		closeErr := file.Close()
		if hashErr != nil {
			return hashErr
		}
		if closeErr != nil {
			return closeErr
		}
		mtime := info.ModTime().UnixMilli()
		files[rel] = model.FileRecord{
			Path:   rel,
			Size:   info.Size(),
			Hash:   hash,
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
	if !filepath.IsLocal(filepath.FromSlash(record.Path)) {
		return fmt.Errorf("invalid relative path %q", record.Path)
	}
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
	localPath := filepath.FromSlash(cleaned)
	if !filepath.IsLocal(localPath) {
		return "", fmt.Errorf("invalid relative path %q", relative)
	}
	baseRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(baseRoot, localPath)
	resolved, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	separator := string(os.PathSeparator)
	if resolved != baseRoot && !strings.HasPrefix(resolved, baseRoot+separator) {
		return "", fmt.Errorf("path %q escapes vault root", relative)
	}
	// Walk each component between baseRoot and resolved to reject symlinked
	// directories that could redirect writes outside the vault.
	rel2, err := filepath.Rel(baseRoot, resolved)
	if err != nil {
		return "", err
	}
	current := baseRoot
	for part := range strings.SplitSeq(rel2, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		fi, lstatErr := os.Lstat(current)
		if lstatErr != nil {
			if os.IsNotExist(lstatErr) {
				break // rest of path does not exist yet; that is fine
			}
			return "", lstatErr
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("path component %q is a symlink", current)
		}
	}
	return resolved, nil
}

func DerivePasswordHash(password, salt string, encryptionVersion int) (string, error) {
	// Match TypeScript: normalize password and salt with NFKC
	normalizedPassword := norm.NFKC.String(password)
	normalizedSalt := norm.NFKC.String(salt)

	// The salt is a hex string representing 16 random bytes.
	// TypeScript uses Buffer.from(normSalt, "utf8") which UTF-8 encodes the hex string.
	// This is different from hex-decoding!
	// Let's match TypeScript behavior.
	rawSalt := []byte(normalizedSalt) // UTF-8 encode the hex string

	// scrypt with N=32768, r=8, p=1, dkLen=32
	rawKey, err := scrypt.Key([]byte(normalizedPassword), rawSalt, 1<<15, 8, 1, 32)
	if err != nil {
		return "", err
	}

	switch encryptionVersion {
	case 2, 3:
		// Match TypeScript computeKeyHash for V2/V3:
		// Web Crypto HKDF derivation
		hkdfReader := hkdf.New(sha256.New, rawKey, rawSalt, []byte("ObsidianKeyHash"))
		keyHash := make([]byte, 32)
		if _, err := hkdfReader.Read(keyHash); err != nil {
			return "", fmt.Errorf("HKDF derivation failed: %w", err)
		}
		return hex.EncodeToString(keyHash), nil
	default:
		// V0: key hash is hex(SHA-256(rawKey))
		hash := sha256.Sum256(rawKey)
		return hex.EncodeToString(hash[:]), nil
	}
}
