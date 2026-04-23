package config

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const AppName = "obsidian-headless"

var (
	ValidFileTypes        = []string{"image", "audio", "video", "pdf", "unsupported"}
	ValidConfigCategories = []string{"app", "appearance", "appearance-data", "hotkey", "core-plugin", "core-plugin-data", "community-plugin", "community-plugin-data"}
)

func BaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "linux" {
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, AppName), nil
		}
		return filepath.Join(home, ".config", AppName), nil
	}
	return filepath.Join(home, "."+AppName), nil
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o700)
}

func authTokenPath() (string, error) {
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "auth_token"), nil
}

func LoadAuthToken() (string, error) {
	if token := strings.TrimSpace(os.Getenv("OBSIDIAN_AUTH_TOKEN")); token != "" {
		return token, nil
	}
	path, err := authTokenPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func SaveAuthToken(token string) error {
	path, err := authTokenPath()
	if err != nil {
		return err
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(token)), 0o600)
}

func ClearAuthToken() error {
	path, err := authTokenPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// MasterKeyPath returns the path to the 32-byte master key used to encrypt
// sensitive values stored in the secrets database.
func MasterKeyPath() (string, error) {
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "master.key"), nil
}

// LoadOrCreateMasterKey loads the 32-byte master encryption key from disk,
// creating a new random key if one does not yet exist.
func LoadOrCreateMasterKey() ([]byte, error) {
	path, err := MasterKeyPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if len(data) != 32 {
			return nil, fmt.Errorf("master.key is corrupt (want 32 bytes, got %d)", len(data))
		}
		return data, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func DefaultDeviceName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s (%s)", host, cases.Title(language.English).String(runtime.GOOS))
}

// validateConfigID rejects vault/site IDs that could be used to escape the
// app config directory when joined into a file path.
func validateConfigID(kind, id string) error {
	if id == "" {
		return fmt.Errorf("%s ID must not be empty", kind)
	}
	// Reject separators and dot-segments that could escape the config tree.
	if strings.ContainsAny(id, `/\`) || id == "." || id == ".." || strings.Contains(id, "..") {
		return fmt.Errorf("invalid %s ID %q", kind, id)
	}
	return nil
}

func SyncDir(vaultID string) (string, error) {
	if err := validateConfigID("vault", vaultID); err != nil {
		return "", err
	}
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "sync", vaultID), nil
}

func PublishDir(siteID string) (string, error) {
	if err := validateConfigID("site", siteID); err != nil {
		return "", err
	}
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "publish", siteID), nil
}

func SyncConfigPath(vaultID string) (string, error) {
	dir, err := SyncDir(vaultID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func PublishConfigPath(siteID string) (string, error) {
	dir, err := PublishDir(siteID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func PublishCachePath(siteID string) (string, error) {
	dir, err := PublishDir(siteID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cache.json"), nil
}

func LogPath(vaultID string) (string, error) {
	dir, err := SyncDir(vaultID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sync.log"), nil
}

func StatePath(vaultID, override string) (string, error) {
	if override != "" {
		return filepath.Clean(override), nil
	}
	dir, err := SyncDir(vaultID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.db"), nil
}

func LockPath(vaultPath, configDir string) string {
	if configDir == "" {
		configDir = ".obsidian"
	}
	return filepath.Join(vaultPath, configDir, ".sync.lock")
}

func WriteSyncConfig(config model.SyncConfig) error {
	path, err := SyncConfigPath(config.VaultID)
	if err != nil {
		return err
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return writeJSON(path, config)
}

func ReadSyncConfig(vaultID string) (*model.SyncConfig, error) {
	path, err := SyncConfigPath(vaultID)
	if err != nil {
		return nil, err
	}
	var cfg model.SyncConfig
	ok, err := readJSON(path, &cfg)
	if err != nil || !ok {
		return nil, err
	}
	return &cfg, nil
}

func RemoveSyncConfig(vaultID string) error {
	dir, err := SyncDir(vaultID)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func WritePublishConfig(config model.PublishConfig) error {
	path, err := PublishConfigPath(config.SiteID)
	if err != nil {
		return err
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return writeJSON(path, config)
}

func ReadPublishConfig(siteID string) (*model.PublishConfig, error) {
	path, err := PublishConfigPath(siteID)
	if err != nil {
		return nil, err
	}
	var cfg model.PublishConfig
	ok, err := readJSON(path, &cfg)
	if err != nil || !ok {
		return nil, err
	}
	return &cfg, nil
}

func WritePublishCache(siteID string, cache map[string]model.PublishCacheEntry) error {
	path, err := PublishCachePath(siteID)
	if err != nil {
		return err
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return writeJSON(path, cache)
}

func ReadPublishCache(siteID string) (map[string]model.PublishCacheEntry, error) {
	path, err := PublishCachePath(siteID)
	if err != nil {
		return nil, err
	}
	cache := map[string]model.PublishCacheEntry{}
	ok, err := readJSON(path, &cache)
	if err != nil {
		return nil, err
	}
	if !ok {
		return map[string]model.PublishCacheEntry{}, nil
	}
	return cache, nil
}

func RemovePublishConfig(siteID string) error {
	dir, err := PublishDir(siteID)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func ListLocalVaults() ([]string, error) {
	base, err := BaseDir()
	if err != nil {
		return nil, err
	}
	return listIDsWithConfig(filepath.Join(base, "sync"))
}

func ListLocalSites() ([]string, error) {
	base, err := BaseDir()
	if err != nil {
		return nil, err
	}
	return listIDsWithConfig(filepath.Join(base, "publish"))
}

func FindSyncConfigByPath(localPath string) (*model.SyncConfig, error) {
	resolved, err := filepath.Abs(localPath)
	if err != nil {
		return nil, err
	}
	ids, err := ListLocalVaults()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		cfg, err := ReadSyncConfig(id)
		if err != nil || cfg == nil {
			continue
		}
		vaultPath, _ := filepath.Abs(cfg.VaultPath)
		if vaultPath == resolved {
			return cfg, nil
		}
	}
	return nil, nil
}

func FindPublishConfigByPath(localPath string) (*model.PublishConfig, error) {
	resolved, err := filepath.Abs(localPath)
	if err != nil {
		return nil, err
	}
	ids, err := ListLocalSites()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		cfg, err := ReadPublishConfig(id)
		if err != nil || cfg == nil {
			continue
		}
		vaultPath, _ := filepath.Abs(cfg.VaultPath)
		if vaultPath == resolved {
			return cfg, nil
		}
	}
	return nil, nil
}

func ParseCSV(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func ValidateChoices(values, valid []string, kind string) error {
	allowed := map[string]struct{}{}
	for _, item := range valid {
		allowed[item] = struct{}{}
	}
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
			return fmt.Errorf("invalid %s %q", kind, value)
		}
	}
	return nil
}

func ValidateConfigDir(dir string) error {
	if dir == "" {
		return nil
	}
	if dir == "." || dir == ".." || filepath.Clean(dir) != dir {
		return fmt.Errorf("config directory must be a single hidden directory name")
	}
	if !strings.HasPrefix(dir, ".") {
		return fmt.Errorf("config directory must start with '.'")
	}
	if strings.ContainsAny(dir, `/\`) {
		return fmt.Errorf("config directory must not contain path separators")
	}
	return nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func readJSON(path string, target any) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return false, err
	}
	return true, nil
}

func listIDsWithConfig(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	result := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, entry.Name(), "config.json")); err == nil {
			result = append(result, entry.Name())
		}
	}
	return result, nil
}
