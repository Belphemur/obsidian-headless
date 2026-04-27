package publish

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"

	"github.com/Belphemur/obsidian-headless/src-go/internal/api"
	configpkg "github.com/Belphemur/obsidian-headless/src-go/internal/config"
	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
	"github.com/Belphemur/obsidian-headless/src-go/internal/util"
)

type Engine struct {
	Client *api.Client
	Config model.PublishConfig
	Token  string
}

type localFile struct {
	Path     string
	FullPath string
	Hash     string
	MTime    int64
	Size     int64
	Publish  *bool
}

type Result struct {
	Uploads []string
	Deletes []string
}

func NewEngine(client *api.Client, config model.PublishConfig, token string) *Engine {
	return &Engine{Client: client, Config: config, Token: token}
}

func (e *Engine) Run(ctx context.Context, dryRun, yes, all bool) (*Result, error) {
	site := model.PublishSite{ID: e.Config.SiteID, Host: e.Config.Host}
	remoteFiles, err := e.Client.ListPublishedFiles(ctx, e.Token, site)
	if err != nil {
		return nil, err
	}
	localFiles, cache, err := e.scanLocal(all)
	if err != nil {
		return nil, err
	}
	remoteMap := map[string]model.PublishFile{}
	for _, file := range remoteFiles {
		remoteMap[file.Path] = file
	}
	result := &Result{}
	for path, file := range localFiles {
		remote, ok := remoteMap[path]
		if !ok || remote.Hash != file.Hash {
			result.Uploads = append(result.Uploads, path)
		}
	}
	for path := range remoteMap {
		if _, ok := localFiles[path]; !ok {
			result.Deletes = append(result.Deletes, path)
		}
	}
	if dryRun {
		return result, nil
	}
	if !yes && (len(result.Uploads) > 0 || len(result.Deletes) > 0) {
		return nil, fmt.Errorf("publishing would change %d files; rerun with --yes or --dry-run", len(result.Uploads)+len(result.Deletes))
	}
	for _, path := range result.Uploads {
		file := localFiles[path]
		content, err := os.ReadFile(file.FullPath)
		if err != nil {
			return nil, err
		}
		if err := e.Client.UploadPublishedFile(ctx, e.Token, site, path, file.Hash, content); err != nil {
			return nil, err
		}
	}
	for _, path := range result.Deletes {
		if err := e.Client.DeletePublishedFile(ctx, e.Token, site, path); err != nil {
			return nil, err
		}
	}
	if err := configpkg.WritePublishCache(e.Config.SiteID, cache); err != nil {
		return nil, err
	}
	return result, nil
}

func (e *Engine) scanLocal(all bool) (map[string]localFile, map[string]model.PublishCacheEntry, error) {
	cache := map[string]model.PublishCacheEntry{}
	files := map[string]localFile{}
	err := filepath.WalkDir(e.Config.VaultPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == e.Config.VaultPath {
			return nil
		}
		rel, err := filepath.Rel(e.Config.VaultPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ".obsidian" || strings.HasPrefix(rel, ".obsidian/") || rel == ".git" || strings.HasPrefix(rel, ".git/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		publishProbe, err := readPublishProbe(path)
		if err != nil {
			return err
		}
		publishFlag := detectPublishFlag(publishProbe)
		if publishFlag != nil && !*publishFlag {
			return nil
		}
		if publishFlag == nil {
			if matchesAny(rel, e.Config.Excludes) {
				return nil
			}
			if len(e.Config.Includes) > 0 && !matchesAny(rel, e.Config.Includes) {
				if !all {
					return nil
				}
			}
			if !all && len(e.Config.Includes) == 0 {
				return nil
			}
		}
		hashFile, err := os.Open(path)
		if err != nil {
			return err
		}
		hash, hashErr := util.HashReader(hashFile)
		closeErr := hashFile.Close()
		if hashErr != nil {
			return hashErr
		}
		if closeErr != nil {
			return closeErr
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files[rel] = localFile{Path: rel, FullPath: path, Hash: hash, MTime: info.ModTime().UnixMilli(), Size: info.Size(), Publish: publishFlag}
		cache[rel] = model.PublishCacheEntry{Hash: hash, MTime: info.ModTime().UnixMilli(), Size: info.Size(), Publish: publishFlag}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return files, cache, nil
}

func matchesAny(path string, patterns []string) bool {
	for _, pattern := range patterns {
		match, err := doublestar.Match(pattern, path)
		if err == nil && match {
			return true
		}
	}
	return false
}

func detectPublishFlag(content []byte) *bool {
	normalized := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return nil
	}
	body := normalized[4:]
	before, _, ok := bytes.Cut(body, []byte("\n---\n"))
	if !ok {
		return nil
	}
	frontmatter := before
	payload := map[string]any{}
	if err := yaml.Unmarshal(frontmatter, &payload); err != nil {
		return nil
	}
	value, ok := payload["publish"]
	if !ok {
		return nil
	}
	boolean, ok := value.(bool)
	if !ok {
		return nil
	}
	return &boolean
}

const publishProbeSize = 16 * 1024

func readPublishProbe(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()
	buffer := make([]byte, publishProbeSize)
	n, err := io.ReadFull(file, buffer)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buffer[:n], nil
}
