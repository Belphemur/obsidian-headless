package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/cenkalti/backoff/v5"
	"github.com/sony/gobreaker/v2"

	"github.com/Belphemur/obsidian-headless/internal/circuitbreaker"
	"github.com/Belphemur/obsidian-headless/internal/model"
)

func (c *Client) listPublishSites(ctx context.Context, token string) ([]model.PublishSite, error) {
	var response struct {
		Sites []model.PublishSite `json:"sites"`
	}
	if err := c.postJSON(ctx, c.apiBase+"/publish/list", map[string]any{"token": token}, &response); err != nil {
		return nil, err
	}
	return response.Sites, nil
}

func (c *Client) createPublishSite(ctx context.Context, token string) (*model.PublishSite, error) {
	var response model.PublishSite
	if err := c.postJSON(ctx, c.apiBase+"/publish/create", map[string]any{"token": token}, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) setPublishSlug(ctx context.Context, token, siteID, host, slug string) error {
	body := map[string]any{"token": token, "id": siteID, "host": host, "slug": slug}
	return c.postJSON(ctx, c.publishAPIBase+"/api/slug", body, nil)
}

func (c *Client) getPublishSlugs(ctx context.Context, token string, ids []string) (map[string]string, error) {
	response := map[string]string{}
	if err := c.postJSON(ctx, c.publishAPIBase+"/api/slugs", map[string]any{"token": token, "ids": ids}, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) listPublishedFiles(ctx context.Context, token string, site model.PublishSite) ([]model.PublishFile, error) {
	var response struct {
		Files []model.PublishFile `json:"files"`
	}
	body := map[string]any{"token": token, "id": site.ID, "version": 2}
	if err := c.postJSON(ctx, hostAPIURL(site.Host, "/api/list"), body, &response); err != nil {
		return nil, err
	}
	return response.Files, nil
}

func (c *Client) uploadPublishedFile(ctx context.Context, token string, site model.PublishSite, path, hash string, content []byte) error {
	operation := func() error {
		_, cbErr := c.cb.Execute(func() (struct{}, error) {
			endpoint := hostAPIURL(site.Host, "/api/upload")
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(content))
			if err != nil {
				return struct{}{}, backoff.Permanent(err)
			}
			req.Header.Set("Content-Type", "application/octet-stream")
			req.Header.Set("User-Agent", userAgent)
			req.Header.Set("obs-token", token)
			req.Header.Set("obs-id", site.ID)
			req.Header.Set("obs-path", url.PathEscape(path))
			req.Header.Set("obs-hash", hash)

			resp, err := c.http.Do(req)
			if err != nil {
				return struct{}{}, backoff.Permanent(err)
			}
			defer resp.Body.Close()

			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				return struct{}{}, backoff.Permanent(err)
			}

			var result map[string]any
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				return struct{}{}, backoff.Permanent(err)
			}
			if code, _ := result["code"].(string); code != "" {
				if msg, _ := result["message"].(string); msg != "" {
					apiErr := &APIError{StatusCode: resp.StatusCode, Message: msg, Code: code}
					if isServerOverloaded(apiErr) {
						return struct{}{}, apiErr
					}
					return struct{}{}, backoff.Permanent(apiErr)
				}
			}
			return struct{}{}, nil
		})

		if errors.Is(cbErr, gobreaker.ErrOpenState) || errors.Is(cbErr, gobreaker.ErrTooManyRequests) {
			return backoff.Permanent(&circuitbreaker.BreakerError{
				Message: "Obsidian API is temporarily unavailable (circuit open); retry in ~30s",
				Err:     cbErr,
			})
		}
		return cbErr
	}

	return c.withRetry(ctx, operation)
}

func (c *Client) deletePublishedFile(ctx context.Context, token string, site model.PublishSite, path string) error {
	body := map[string]any{"token": token, "id": site.ID, "path": path}
	return c.postJSON(ctx, hostAPIURL(site.Host, "/api/remove"), body, nil)
}
