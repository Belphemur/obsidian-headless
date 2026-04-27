package api

import (
	"context"

	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
)

func (c *Client) regions(ctx context.Context, token string) ([]model.Region, error) {
	var response struct {
		Regions []model.Region `json:"regions"`
	}
	if err := c.postJSON(ctx, c.apiBase+"/vault/regions", map[string]any{"token": token}, &response); err != nil {
		return nil, err
	}
	return response.Regions, nil
}

func (c *Client) listVaults(ctx context.Context, token string, supportedVersion int) ([]model.Vault, error) {
	var response struct {
		Vaults []model.Vault `json:"vaults"`
	}
	body := map[string]any{"token": token, "supported_encryption_version": supportedVersion}
	if err := c.postJSON(ctx, c.apiBase+"/vault/list", body, &response); err != nil {
		return nil, err
	}
	return response.Vaults, nil
}

func (c *Client) createVault(ctx context.Context, token, name, keyHash, salt, region string, encryptionVersion int) (*model.Vault, error) {
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

func (c *Client) validateVaultAccess(ctx context.Context, token, vaultID, keyHash, host string, supportedVersion int) error {
	body := map[string]any{
		"token":                        token,
		"vault_uid":                    vaultID,
		"host":                         host,
		"supported_encryption_version": supportedVersion,
		"encryption_version":           supportedVersion,
	}
	if keyHash != "" {
		body["keyhash"] = keyHash
	}
	return c.postJSON(ctx, c.apiBase+"/vault/access", body, nil)
}
