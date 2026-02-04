package errors

import (
	"fmt"
	"net/http"
)

// ErrorCode represents an error code
type ErrorCode string

const (
	// General errors
	ErrCodeInternal     ErrorCode = "INTERNAL_ERROR"
	ErrCodeInvalidInput ErrorCode = "INVALID_INPUT"
	ErrCodeNotFound     ErrorCode = "NOT_FOUND"
	ErrCodeUnauthorized ErrorCode = "UNAUTHORIZED"
	ErrCodeForbidden    ErrorCode = "FORBIDDEN"
	ErrCodeConflict     ErrorCode = "CONFLICT"
	ErrCodeRateLimited  ErrorCode = "RATE_LIMITED"

	// Room errors
	ErrCodeRoomNotFound      ErrorCode = "ROOM_NOT_FOUND"
	ErrCodeRoomFull          ErrorCode = "ROOM_FULL"
	ErrCodeRoomAlreadyExists ErrorCode = "ROOM_ALREADY_EXISTS"

	// Node errors
	ErrCodeNodeNotFound      ErrorCode = "NODE_NOT_FOUND"
	ErrCodeNodeOverloaded    ErrorCode = "NODE_OVERLOADED"
	ErrCodeNodeUnhealthy     ErrorCode = "NODE_UNHEALTHY"
	ErrCodeNodeAlreadyExists ErrorCode = "NODE_ALREADY_EXISTS"

	// Stream errors
	ErrCodeStreamNotFound       ErrorCode = "STREAM_NOT_FOUND"
	ErrCodeStreamAlreadyExists  ErrorCode = "STREAM_ALREADY_EXISTS"
	ErrCodeStreamNotAvailable   ErrorCode = "STREAM_NOT_AVAILABLE"
	ErrCodeStreamRecoveryFailed ErrorCode = "STREAM_RECOVERY_FAILED"

	// Network errors
	ErrCodeNetworkError      ErrorCode = "NETWORK_ERROR"
	ErrCodeConnectionFailed  ErrorCode = "CONNECTION_FAILED"
	ErrCodeConnectionTimeout ErrorCode = "CONNECTION_TIMEOUT"
	ErrCodeHighLatency       ErrorCode = "HIGH_LATENCY"
	ErrCodeHighPacketLoss    ErrorCode = "HIGH_PACKET_LOSS"

	// Resource errors
	ErrCodeInsufficientResources ErrorCode = "INSUFFICIENT_RESOURCES"
	ErrCodeInsufficientBandwidth ErrorCode = "INSUFFICIENT_BANDWIDTH"
	ErrCodeInsufficientCPU       ErrorCode = "INSUFFICIENT_CPU"
	ErrCodeInsufficientMemory    ErrorCode = "INSUFFICIENT_MEMORY"

	// Authentication/Authorization errors
	ErrCodeAuthenticationFailed ErrorCode = "AUTHENTICATION_FAILED"
	ErrCodeAuthorizationFailed  ErrorCode = "AUTHORIZATION_FAILED"
	ErrCodeInvalidToken         ErrorCode = "INVALID_TOKEN"
	ErrCodeTokenExpired         ErrorCode = "TOKEN_EXPIRED"

	// Protocol errors
	ErrCodeInvalidMessage    ErrorCode = "INVALID_MESSAGE"
	ErrCodeInvalidRole       ErrorCode = "INVALID_ROLE"
	ErrCodeInvalidStreamType ErrorCode = "INVALID_STREAM_TYPE"

	// Configuration errors
	ErrCodeInvalidConfig  ErrorCode = "INVALID_CONFIG"
	ErrCodeConfigNotFound ErrorCode = "CONFIG_NOT_FOUND"
)

// AppError represents an application error
type AppError struct {
	Code       ErrorCode              `json:"code"`
	Message    string                 `json:"message"`
	HTTPStatus int                    `json:"-"`
	Details    map[string]interface{} `json:"details,omitempty"`
	Cause      error                  `json:"-"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *AppError) Unwrap() error {
	return e.Cause
}

// WithDetails adds details to the error
func (e *AppError) WithDetails(key string, value interface{}) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// WithCause sets the underlying cause
func (e *AppError) WithCause(cause error) *AppError {
	e.Cause = cause
	return e
}

// NewAppError creates a new application error
func NewAppError(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: getHTTPStatus(code),
	}
}

// NewAppErrorf creates a new application error with formatting
func NewAppErrorf(code ErrorCode, format string, args ...interface{}) *AppError {
	return &AppError{
		Code:       code,
		Message:    fmt.Sprintf(format, args...),
		HTTPStatus: getHTTPStatus(code),
	}
}

// getHTTPStatus returns the HTTP status code for an error code
func getHTTPStatus(code ErrorCode) int {
	switch code {
	case ErrCodeNotFound, ErrCodeRoomNotFound, ErrCodeNodeNotFound, ErrCodeStreamNotFound:
		return http.StatusNotFound
	case ErrCodeUnauthorized, ErrCodeAuthenticationFailed, ErrCodeInvalidToken, ErrCodeTokenExpired:
		return http.StatusUnauthorized
	case ErrCodeForbidden, ErrCodeAuthorizationFailed:
		return http.StatusForbidden
	case ErrCodeConflict, ErrCodeRoomAlreadyExists, ErrCodeNodeAlreadyExists, ErrCodeStreamAlreadyExists:
		return http.StatusConflict
	case ErrCodeRateLimited:
		return http.StatusTooManyRequests
	case ErrCodeInvalidInput, ErrCodeInvalidMessage, ErrCodeInvalidRole, ErrCodeInvalidStreamType, ErrCodeInvalidConfig:
		return http.StatusBadRequest
	case ErrCodeRoomFull, ErrCodeNodeOverloaded, ErrCodeInsufficientResources, ErrCodeInsufficientBandwidth, ErrCodeInsufficientCPU, ErrCodeInsufficientMemory:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// IsAppError checks if an error is an AppError
func IsAppError(err error) bool {
	_, ok := err.(*AppError)
	return ok
}

// AsAppError converts an error to AppError
func AsAppError(err error) (*AppError, bool) {
	appErr, ok := err.(*AppError)
	return appErr, ok
}

// WrapError wraps a standard error as an AppError
func WrapError(code ErrorCode, err error) *AppError {
	return &AppError{
		Code:       code,
		Message:    err.Error(),
		HTTPStatus: getHTTPStatus(code),
		Cause:      err,
	}
}
