package errors

import (
	"errors"
	"fmt"
	"net/http"
)

type AppError struct {
	HTTPStatus int `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Internal   error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Internal)
	}
	return e.Message
}

func (e *AppError) Unwrap() error { return e.Internal }

func New(status int, code, message string) *AppError {
	return &AppError{HTTPStatus: status, Code: code, Message: message}
}

func Wrap(status int, code, message string, internal error) *AppError {
	return &AppError{HTTPStatus: status, Code: code, Message: message, Internal: internal}
}

var (
	// Auth / access errors
	ErrUnauthorized          = New(http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
	ErrForbidden             = New(http.StatusForbidden, "FORBIDDEN", "You do not have permission to access this resource")
	ErrTokenExpired          = New(http.StatusUnauthorized, "TOKEN_EXPIRED", "Access token has expired")
	ErrTokenInvalid          = New(http.StatusUnauthorized, "TOKEN_INVALID", "Access token is invalid")
	ErrReplayDetected        = New(http.StatusUnauthorized, "REPLAY_DETECTED", "Duplicate request detected; include a fresh X-Nonce and X-Request-Timestamp")
	ErrTimestampInvalid      = New(http.StatusUnauthorized, "TIMESTAMP_INVALID", "Request timestamp is outside the allowed window")
	ErrMissingSecurityHeader = New(http.StatusBadRequest, "MISSING_SECURITY_HEADER", "Required security header (X-Nonce or X-Request-Timestamp) is missing")

	// Resource errors
	ErrNotFound = New(http.StatusNotFound, "NOT_FOUND", "The requested resource was not found")
	ErrConflict = New(http.StatusConflict, "CONFLICT", "A resource with that identifier already exists")

	// Quota / subscription errors
	ErrQuotaExhausted        = New(http.StatusPaymentRequired, "QUOTA_EXHAUSTED", "Monthly free quota exhausted; please subscribe to continue")
	ErrNoActiveSubscription  = New(http.StatusPaymentRequired, "NO_ACTIVE_SUBSCRIPTION", "No active subscription with remaining messages found")
	ErrSubscriptionInactive  = New(http.StatusBadRequest, "SUBSCRIPTION_INACTIVE", "The subscription is not active")
	ErrSubscriptionCancelled = New(http.StatusBadRequest, "SUBSCRIPTION_CANCELLED", "The subscription has been cancelled")

	// Validation errors
	ErrValidation   = New(http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Request validation failed")
	ErrInvalidInput = New(http.StatusBadRequest, "INVALID_INPUT", "Invalid request input")
	ErrBodyTooLarge = New(http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", "Request body exceeds the maximum allowed size")
	ErrContentType  = New(http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json")

	// Rate limiting
	ErrRateLimited = New(http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests; please slow down")

	// Generic server error
	ErrInternal = New(http.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred")
)


func IsAppError(err error) bool {
	var ae *AppError
	return errors.As(err, &ae)
}

func ToAppError(err error) *AppError {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae
	}
	return Wrap(http.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred", err)
}

func ValidationError(detail string) *AppError {
	return New(http.StatusUnprocessableEntity, "VALIDATION_ERROR", detail)
}
