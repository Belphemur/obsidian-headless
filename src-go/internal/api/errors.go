package api

import (
	"errors"
	"strings"
)

type apiError struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// APIError represents an error response from the Obsidian API.
type APIError struct {
	StatusCode int
	Message    string
	Code       string
}

func (e *APIError) Error() string {
	return e.Message
}

// RequestOptions allows customizing individual API requests.
type RequestOptions struct {
	Headers map[string]string
}

func makeAPIError(statusCode int, status string, appErr apiError, target any) error {
	if appErr.Error != "" {
		return &APIError{StatusCode: statusCode, Message: appErr.Error, Code: appErr.Code}
	}
	if statusCode >= 400 {
		message := appErr.Message
		if message == "" {
			message = status
		}
		return &APIError{StatusCode: statusCode, Message: message, Code: appErr.Code}
	}
	if target == nil {
		if appErr.Message != "" {
			return &APIError{StatusCode: statusCode, Message: appErr.Message, Code: appErr.Code}
		}
		return nil
	}
	if appErr.Message != "" && appErr.Code != "" {
		return &APIError{StatusCode: statusCode, Message: appErr.Message, Code: appErr.Code}
	}
	return nil
}

func isServerOverloaded(err error) bool {
	if apiErr, ok := errors.AsType[*APIError](err); ok {
		return strings.Contains(strings.ToLower(apiErr.Message), "overloaded")
	}
	return false
}
