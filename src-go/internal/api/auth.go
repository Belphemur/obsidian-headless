package api

import (
	"context"

	"github.com/Belphemur/obsidian-headless/src-go/internal/model"
)

func (c *Client) signIn(ctx context.Context, email, password, mfa string) (*model.SignInResponse, error) {
	body := map[string]any{"email": email, "password": password}
	if mfa != "" {
		body["mfa"] = mfa
	}

	var response model.SignInResponse
	if err := c.postJSON(ctx, c.apiBase+"/user/signin", body, &response, &RequestOptions{
		Headers: map[string]string{"Origin": "https://obsidian.md"},
	}); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) signOut(ctx context.Context, token string) error {
	return c.postJSON(ctx, c.apiBase+"/user/signout", map[string]any{"token": token}, nil)
}

func (c *Client) userInfo(ctx context.Context, token string) (*model.UserInfo, error) {
	var response model.UserInfo
	if err := c.postJSON(ctx, c.apiBase+"/user/info", map[string]any{"token": token}, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
