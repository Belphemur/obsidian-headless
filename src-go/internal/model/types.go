package model

type UserInfo struct {
UID      string `json:"uid"`
Name     string `json:"name"`
Email    string `json:"email"`
MFA      bool   `json:"mfa"`
License  string `json:"license"`
Credit   int    `json:"credit"`
Discount int    `json:"discount"`
}

type SignInResponse struct {
Token string `json:"token"`
Name  string `json:"name"`
Email string `json:"email"`
}

type Region struct {
ID   string `json:"id"`
Name string `json:"name"`
}

type Vault struct {
ID                string `json:"id"`
UID               string `json:"uid"`
Name              string `json:"name"`
Password          string `json:"password"`
Salt              string `json:"salt"`
Created           int64  `json:"created"`
Host              string `json:"host"`
Size              int64  `json:"size"`
EncryptionVersion int    `json:"encryption_version"`
}

type PublishSite struct {
ID      string `json:"id"`
Slug    string `json:"slug"`
Host    string `json:"host"`
Created int64  `json:"created"`
}

type PublishFile struct {
Path string `json:"path"`
Hash string `json:"hash"`
Size int64  `json:"size"`
}

type SyncConfig struct {
VaultID            string   `json:"vaultId"`
VaultName          string   `json:"vaultName"`
VaultPath          string   `json:"vaultPath"`
Host               string   `json:"host"`
EncryptionVersion  int      `json:"encryptionVersion"`
EncryptionKey      string   `json:"encryptionKey"`
EncryptionSalt     string   `json:"encryptionSalt"`
ConflictStrategy   string   `json:"conflictStrategy"`
SyncMode           string   `json:"syncMode,omitempty"`
DeviceName         string   `json:"deviceName,omitempty"`
ConfigDir          string   `json:"configDir,omitempty"`
AllowTypes         []string `json:"allowTypes,omitempty"`
AllowSpecialFiles  []string `json:"allowSpecialFiles,omitempty"`
IgnoreFolders      []string `json:"ignoreFolders,omitempty"`
StatePath          string   `json:"statePath,omitempty"`
}

type PublishConfig struct {
SiteID    string   `json:"siteId"`
Host      string   `json:"host"`
VaultPath string   `json:"vaultPath"`
Includes  []string `json:"includes,omitempty"`
Excludes  []string `json:"excludes,omitempty"`
}

type FileRecord struct {
Path         string `json:"path"`
PreviousPath string `json:"previouspath,omitempty"`
Size         int64  `json:"size"`
Hash         string `json:"hash"`
CTime        int64  `json:"ctime"`
MTime        int64  `json:"mtime"`
Folder       bool   `json:"folder"`
Deleted      bool   `json:"deleted,omitempty"`
UID          int64  `json:"uid,omitempty"`
Device       string `json:"device,omitempty"`
User         string `json:"user,omitempty"`
}

type PublishCacheEntry struct {
Hash    string `json:"hash"`
MTime   int64  `json:"mtime"`
Size    int64  `json:"size"`
Publish *bool  `json:"publish,omitempty"`
}
