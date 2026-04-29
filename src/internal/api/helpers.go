package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/cenkalti/backoff/v5"
)

func (c *Client) postJSON(ctx context.Context, endpoint string, body any, target any, opts ...*RequestOptions) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	operation := func() error {
		// Always send OPTIONS preflight first (matches TypeScript client behavior)
		preflightReq, _ := http.NewRequestWithContext(ctx, http.MethodOptions, endpoint, nil)
		if resp, err := c.http.Do(preflightReq); err == nil {
			resp.Body.Close()
		}

		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return backoff.Permanent(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("User-Agent", userAgent)
		if len(opts) > 0 && opts[0] != nil && opts[0].Headers != nil {
			for k, v := range opts[0].Headers {
				request.Header.Set(k, v)
			}
		}

		response, err := c.http.Do(request)
		if err != nil {
			return backoff.Permanent(err)
		}
		defer func() {
			_ = response.Body.Close()
		}()

		bodyBytes, err := io.ReadAll(response.Body)
		if err != nil {
			return backoff.Permanent(err)
		}

		var appErr apiError
		_ = json.Unmarshal(bodyBytes, &appErr)

		if apiErr := makeAPIError(response.StatusCode, response.Status, appErr, target); apiErr != nil {
			if isServerOverloaded(apiErr) {
				return apiErr
			}
			return backoff.Permanent(apiErr)
		}

		if target == nil {
			return nil
		}
		return json.Unmarshal(bodyBytes, target)
	}

	return c.withRetry(ctx, operation)
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
