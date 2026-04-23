package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
)

type Client struct {
	apiBase string
	http    *http.Client
}

type apiError struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func New(apiBase string, timeout time.Duration) *Client {
	if apiBase == "" {
		apiBase = "https://api.obsidian.md"
	}
	return &Client{
		apiBase: strings.TrimRight(apiBase, "/"),
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *Client) SignIn(ctx context.Context, email, password, mfa string) (*model.SignInResponse, error) {
	body := map[string]any{"email": email, "password": password}
	if mfa != "" {
		body["mfa"] = mfa
	}
	var response model.SignInResponse
	if err := c.postJSON(ctx, c.apiBase+"/user/signin", body, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) SignOut(ctx context.Context, token string) error {
	return c.postJSON(ctx, c.apiBase+"/user/signout", map[string]any{"token": token}, nil)
}

func (c *Client) UserInfo(ctx context.Context, token string) (*model.UserInfo, error) {
	var response model.UserInfo
	if err := c.postJSON(ctx, c.apiBase+"/user/info", map[string]any{"token": token}, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) Regions(ctx context.Context, token string) ([]model.Region, error) {
	var response struct {
		Regions []model.Region `json:"regions"`
	}
	if err := c.postJSON(ctx, c.apiBase+"/vault/regions", map[string]any{"token": token}, &response); err != nil {
		return nil, err
	}
	return response.Regions, nil
}

func (c *Client) ListVaults(ctx context.Context, token string, supportedVersion int) ([]model.Vault, error) {
	var response struct {
		Vaults []model.Vault `json:"vaults"`
	}
	body := map[string]any{"token": token, "supported_encryption_version": supportedVersion}
	if err := c.postJSON(ctx, c.apiBase+"/vault/list", body, &response); err != nil {
		return nil, err
	}
	return response.Vaults, nil
}

func (c *Client) CreateVault(ctx context.Context, token, name, keyHash, salt, region string, encryptionVersion int) (*model.Vault, error) {
	body := map[string]any{
		"token":              token,
		"name":               name,
		"keyhash":            keyHash,
		"salt":               salt,
		"encryption_version": encryptionVersion,
		"region":             region,
	}
	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.postJSON(ctx, c.apiBase+"/vault/create", body, &created); err != nil {
		return nil, err
	}
	vaults, err := c.ListVaults(ctx, token, encryptionVersion)
	if err != nil {
		return nil, err
	}
	for _, vault := range vaults {
		if vault.ID == created.ID {
			return &vault, nil
		}
	}
	return &model.Vault{ID: created.ID, UID: created.ID, Name: created.Name}, nil
}

func (c *Client) ValidateVaultAccess(ctx context.Context, token, vaultID, keyHash, host string, supportedVersion int) error {
	body := map[string]any{
		"token":                        token,
		"uid":                          vaultID,
		"keyhash":                      keyHash,
		"host":                         host,
		"supported_encryption_version": supportedVersion,
	}
	return c.postJSON(ctx, c.apiBase+"/vault/access", body, nil)
}

func (c *Client) ListPublishSites(ctx context.Context, token string) ([]model.PublishSite, error) {
	var response struct {
		Sites []model.PublishSite `json:"sites"`
	}
	if err := c.postJSON(ctx, c.apiBase+"/publish/list", map[string]any{"token": token}, &response); err != nil {
		return nil, err
	}
	return response.Sites, nil
}

func (c *Client) CreatePublishSite(ctx context.Context, token string) (*model.PublishSite, error) {
	var response model.PublishSite
	if err := c.postJSON(ctx, c.apiBase+"/publish/create", map[string]any{"token": token}, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) SetPublishSlug(ctx context.Context, token, siteID, host, slug string) error {
	body := map[string]any{"token": token, "id": siteID, "host": host, "slug": slug}
	return c.postJSON(ctx, hostAPIURL(host, "/api/slug"), body, nil)
}

func (c *Client) GetPublishSlugs(ctx context.Context, token string, ids []string) (map[string]string, error) {
	response := map[string]string{}
	if err := c.postJSON(ctx, c.apiBase+"/api/slugs", map[string]any{"token": token, "ids": ids}, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) ListPublishedFiles(ctx context.Context, token string, site model.PublishSite) ([]model.PublishFile, error) {
	var response struct {
		Files []model.PublishFile `json:"files"`
	}
	body := map[string]any{"token": token, "id": site.ID, "host": site.Host}
	if err := c.postJSON(ctx, hostAPIURL(site.Host, "/api/list"), body, &response); err != nil {
		return nil, err
	}
	return response.Files, nil
}

func (c *Client) UploadPublishedFile(ctx context.Context, token string, site model.PublishSite, path, hash string, content []byte) error {
	body := map[string]any{
		"token":   token,
		"id":      site.ID,
		"host":    site.Host,
		"path":    path,
		"hash":    hash,
		"content": base64.StdEncoding.EncodeToString(content),
	}
	return c.postJSON(ctx, hostAPIURL(site.Host, "/api/put"), body, nil)
}

func (c *Client) DeletePublishedFile(ctx context.Context, token string, site model.PublishSite, path string) error {
	body := map[string]any{"token": token, "id": site.ID, "host": site.Host, "path": path}
	return c.postJSON(ctx, hostAPIURL(site.Host, "/api/delete"), body, nil)
}

func (c *Client) postJSON(ctx context.Context, endpoint string, body any, target any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		var apiErr apiError
		_ = json.NewDecoder(response.Body).Decode(&apiErr)
		message := apiErr.Error
		if message == "" {
			message = apiErr.Message
		}
		if message == "" {
			message = response.Status
		}
		return fmt.Errorf("%s", message)
	}
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	// Always check for an application-level error payload first.
	var appErr apiError
	if decErr := json.Unmarshal(bodyBytes, &appErr); decErr == nil {
		if appErr.Error != "" {
			return fmt.Errorf("%s", appErr.Error)
		}
		if appErr.Message != "" && appErr.Code != "" {
			return fmt.Errorf("%s", appErr.Message)
		}
	}

	if target == nil {
		return nil
	}
	return json.Unmarshal(bodyBytes, target)
}

func hostAPIURL(host, path string) string {
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		return strings.TrimRight(host, "/") + path
	}
	hostname := host
	if parsed, err := url.Parse(host); err == nil && parsed.Host != "" {
		hostname = parsed.Host
	}
	protocol := "https://"
	if strings.HasPrefix(hostname, "localhost") || strings.HasPrefix(hostname, "127.0.0.1") {
		protocol = "http://"
	}
	return protocol + strings.TrimRight(hostname, "/") + path
}
