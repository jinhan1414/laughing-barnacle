package llmgateway

import (
	"errors"
	"fmt"
	"strings"
)

type ErrorCode string

const (
	ErrorCodeInvalidRequest        ErrorCode = "invalid_request"
	ErrorCodeProviderNotRegistered ErrorCode = "provider_not_registered"
	ErrorCodeProviderRequestFailed ErrorCode = "provider_request_failed"
	ErrorCodeAuthConfigInvalid     ErrorCode = "auth_config_invalid"
)

type Error struct {
	Code       ErrorCode
	Provider   string
	Message    string
	StatusCode int
	Cause      error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	parts := make([]string, 0, 4)
	if e.Code != "" {
		parts = append(parts, string(e.Code))
	}
	if p := strings.TrimSpace(e.Provider); p != "" {
		parts = append(parts, "provider="+p)
	}
	if m := strings.TrimSpace(e.Message); m != "" {
		parts = append(parts, m)
	}
	if e.StatusCode > 0 {
		parts = append(parts, fmt.Sprintf("status=%d", e.StatusCode))
	}
	if e.Cause != nil {
		parts = append(parts, e.Cause.Error())
	}
	return strings.Join(parts, ": ")
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func WrapProviderError(provider string, err error) error {
	if err == nil {
		return nil
	}
	var gatewayErr *Error
	if errors.As(err, &gatewayErr) {
		if gatewayErr.Provider == "" {
			gatewayErr.Provider = provider
		}
		return gatewayErr
	}
	return &Error{
		Code:     ErrorCodeProviderRequestFailed,
		Provider: provider,
		Message:  "provider call failed",
		Cause:    err,
	}
}
