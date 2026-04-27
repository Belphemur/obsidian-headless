package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
)

const userAgent = "node"

// Client is an HTTP client for the Obsidian REST API.
type Client struct {
	apiBase        string
	publishAPIBase string
	http           *http.Client
}

// New creates a new API client.
func New(apiBase string, timeout time.Duration) *Client {
	if apiBase == "" {
		apiBase = "https://api.obsidian.md"
	}
	publishAPIBase := "https://publish.obsidian.md"
	// Use same base for local testing
	if strings.Contains(apiBase, "127.0.0.1") || strings.Contains(apiBase, "localhost") {
		publishAPIBase = apiBase
	}
	return &Client{
		apiBase:        strings.TrimRight(apiBase, "/"),
		publishAPIBase: strings.TrimRight(publishAPIBase, "/"),
		http:           &http.Client{Timeout: timeout},
	}
}

// SignIn authenticates a user.
func (c *Client) SignIn(ctx context.Context, email, password, mfa string) (*model.SignInResponse, error) {
	return c.signIn(ctx, email, password, mfa)
}

// SignOut invalidates a token.
func (c *Client) SignOut(ctx context.Context, token string) error {
	return c.signOut(ctx, token)
}

// UserInfo returns information about the authenticated user.
func (c *Client) UserInfo(ctx context.Context, token string) (*model.UserInfo, error) {
	return c.userInfo(ctx, token)
}

// Regions returns available vault regions.
func (c *Client) Regions(ctx context.Context, token string) ([]model.Region, error) {
	return c.regions(ctx, token)
}

// ListVaults returns the vaults accessible by the token.
func (c *Client) ListVaults(ctx context.Context, token string, supportedVersion int) ([]model.Vault, error) {
	return c.listVaults(ctx, token, supportedVersion)
}

// CreateVault creates a new vault.
func (c *Client) CreateVault(ctx context.Context, token, name, keyHash, salt, region string, encryptionVersion int) (*model.Vault, error) {
	return c.createVault(ctx, token, name, keyHash, salt, region, encryptionVersion)
}

// ValidateVaultAccess validates access to a vault.
func (c *Client) ValidateVaultAccess(ctx context.Context, token, vaultID, keyHash, host string, supportedVersion int) error {
	return c.validateVaultAccess(ctx, token, vaultID, keyHash, host, supportedVersion)
}

// ListPublishSites returns the publish sites for the user.
func (c *Client) ListPublishSites(ctx context.Context, token string) ([]model.PublishSite, error) {
	return c.listPublishSites(ctx, token)
}

// CreatePublishSite creates a new publish site.
func (c *Client) CreatePublishSite(ctx context.Context, token string) (*model.PublishSite, error) {
	return c.createPublishSite(ctx, token)
}

// SetPublishSlug sets the slug for a publish site.
func (c *Client) SetPublishSlug(ctx context.Context, token, siteID, host, slug string) error {
	return c.setPublishSlug(ctx, token, siteID, host, slug)
}

// GetPublishSlugs returns slugs for the given site IDs.
func (c *Client) GetPublishSlugs(ctx context.Context, token string, ids []string) (map[string]string, error) {
	return c.getPublishSlugs(ctx, token, ids)
}

// ListPublishedFiles returns the published files for a site.
func (c *Client) ListPublishedFiles(ctx context.Context, token string, site model.PublishSite) ([]model.PublishFile, error) {
	return c.listPublishedFiles(ctx, token, site)
}

// UploadPublishedFile uploads a file to a publish site.
func (c *Client) UploadPublishedFile(ctx context.Context, token string, site model.PublishSite, path, hash string, content []byte) error {
	return c.uploadPublishedFile(ctx, token, site, path, hash, content)
}

// DeletePublishedFile removes a file from a publish site.
func (c *Client) DeletePublishedFile(ctx context.Context, token string, site model.PublishSite, path string) error {
	return c.deletePublishedFile(ctx, token, site, path)
}
