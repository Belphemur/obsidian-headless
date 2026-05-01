package api

import (
	"errors"
	"testing"
)

func TestAPIError_Error(t *testing.T) {
	t.Parallel()
	err := &APIError{StatusCode: 500, Message: "internal error", Code: "ERR"}
	if got := err.Error(); got != "internal error" {
		t.Errorf("Error() = %q, want %q", got, "internal error")
	}
}

func TestMakeAPIError_ErrorField(t *testing.T) {
	t.Parallel()
	appErr := apiError{Error: "something went wrong", Code: "CODE"}
	err := makeAPIError(400, "Bad Request", appErr, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Message != "something went wrong" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "something went wrong")
	}
	if apiErr.Code != "CODE" {
		t.Errorf("Code = %q, want %q", apiErr.Code, "CODE")
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, 400)
	}
}

func TestMakeAPIError_StatusCode400(t *testing.T) {
	t.Parallel()
	appErr := apiError{Message: "msg", Code: "C"}
	err := makeAPIError(400, "Bad Request", appErr, &struct{}{})
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr := err.(*APIError)
	if apiErr.Message != "msg" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "msg")
	}
}

func TestMakeAPIError_StatusCode400NoMessage(t *testing.T) {
	t.Parallel()
	appErr := apiError{}
	err := makeAPIError(404, "Not Found", appErr, &struct{}{})
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr := err.(*APIError)
	if apiErr.Message != "Not Found" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "Not Found")
	}
}

func TestMakeAPIError_TargetNilWithMessage(t *testing.T) {
	t.Parallel()
	appErr := apiError{Message: "m", Code: "c"}
	err := makeAPIError(200, "OK", appErr, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr := err.(*APIError)
	if apiErr.Message != "m" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "m")
	}
}

func TestMakeAPIError_TargetNilNoMessage(t *testing.T) {
	t.Parallel()
	appErr := apiError{}
	err := makeAPIError(200, "OK", appErr, nil)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestMakeAPIError_TargetNotNilMessageAndCode(t *testing.T) {
	t.Parallel()
	appErr := apiError{Message: "m", Code: "c"}
	err := makeAPIError(200, "OK", appErr, &struct{}{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMakeAPIError_TargetNotNilNoMessageOrCode(t *testing.T) {
	t.Parallel()
	appErr := apiError{}
	err := makeAPIError(200, "OK", appErr, &struct{}{})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestIsServerOverloaded_True(t *testing.T) {
	t.Parallel()
	err := &APIError{Message: "Server is overloaded, please retry"}
	if !isServerOverloaded(err) {
		t.Error("expected true")
	}
}

func TestIsServerOverloaded_False(t *testing.T) {
	t.Parallel()
	err := &APIError{Message: "Not found"}
	if isServerOverloaded(err) {
		t.Error("expected false")
	}
}

func TestIsServerOverloaded_NonAPIError(t *testing.T) {
	t.Parallel()
	if isServerOverloaded(errors.New("some error")) {
		t.Error("expected false")
	}
}
