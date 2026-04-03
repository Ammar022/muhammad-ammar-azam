package unit

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/Ammar022/secure-ai-chat-backend/internal/shared/errors"
)

// ── AppError.Error() ──────────────────────────────────────────────────────────

func TestAppError_Error_WithNoInternal(t *testing.T) {
	err := apperrors.New(http.StatusBadRequest, "BAD", "bad request")
	assert.Equal(t, "bad request", err.Error())
}

func TestAppError_Error_WithInternal_IncludesInternalMessage(t *testing.T) {
	internal := errors.New("db connection refused")
	err := apperrors.Wrap(http.StatusInternalServerError, "DB_ERR", "something went wrong", internal)
	assert.Contains(t, err.Error(), "something went wrong")
	assert.Contains(t, err.Error(), "db connection refused")
}

// ── AppError.Unwrap() ─────────────────────────────────────────────────────────

func TestAppError_Unwrap_ReturnsInternal(t *testing.T) {
	internal := errors.New("root cause")
	err := apperrors.Wrap(http.StatusInternalServerError, "X", "msg", internal)
	assert.ErrorIs(t, err, internal)
}

func TestAppError_Unwrap_NilWhenNoInternal(t *testing.T) {
	err := apperrors.New(http.StatusBadRequest, "X", "msg")
	assert.Nil(t, err.Unwrap())
}

// ── IsAppError ────────────────────────────────────────────────────────────────

func TestIsAppError_WithAppError_ReturnsTrue(t *testing.T) {
	err := apperrors.New(http.StatusNotFound, "NOT_FOUND", "not found")
	assert.True(t, apperrors.IsAppError(err))
}

func TestIsAppError_WithWrappedAppError_ReturnsTrue(t *testing.T) {
	inner := apperrors.New(http.StatusBadRequest, "BAD", "bad")
	wrapped := fmt.Errorf("outer: %w", inner)
	assert.True(t, apperrors.IsAppError(wrapped))
}

func TestIsAppError_WithPlainError_ReturnsFalse(t *testing.T) {
	err := errors.New("plain error")
	assert.False(t, apperrors.IsAppError(err))
}

func TestIsAppError_WithNil_ReturnsFalse(t *testing.T) {
	assert.False(t, apperrors.IsAppError(nil))
}

// ── ToAppError ────────────────────────────────────────────────────────────────

func TestToAppError_WithAppError_ReturnsSameError(t *testing.T) {
	original := apperrors.New(http.StatusUnauthorized, "UNAUTH", "unauthorized")
	result := apperrors.ToAppError(original)
	assert.Equal(t, original.HTTPStatus, result.HTTPStatus)
	assert.Equal(t, original.Code, result.Code)
	assert.Equal(t, original.Message, result.Message)
}

func TestToAppError_WithPlainError_WrapsAsInternal(t *testing.T) {
	plain := errors.New("unexpected db error")
	result := apperrors.ToAppError(plain)
	assert.Equal(t, http.StatusInternalServerError, result.HTTPStatus)
	assert.Equal(t, "INTERNAL_ERROR", result.Code)
	assert.Equal(t, plain, result.Internal)
}

func TestToAppError_WithWrappedAppError_ExtractsAppError(t *testing.T) {
	inner := apperrors.New(http.StatusForbidden, "FORBIDDEN", "forbidden")
	wrapped := fmt.Errorf("context: %w", inner)
	result := apperrors.ToAppError(wrapped)
	assert.Equal(t, http.StatusForbidden, result.HTTPStatus)
	assert.Equal(t, "FORBIDDEN", result.Code)
}

// ── ValidationError ───────────────────────────────────────────────────────────

func TestValidationError_Has422Status(t *testing.T) {
	err := apperrors.ValidationError("field 'email' is required")
	assert.Equal(t, http.StatusUnprocessableEntity, err.HTTPStatus)
}

func TestValidationError_HasValidationCode(t *testing.T) {
	err := apperrors.ValidationError("invalid input")
	assert.Equal(t, "VALIDATION_ERROR", err.Code)
}

func TestValidationError_MessageIsDetail(t *testing.T) {
	detail := "field 'tier' must be one of: basic, pro, enterprise"
	err := apperrors.ValidationError(detail)
	assert.Equal(t, detail, err.Message)
}

// ── Sentinel error properties ─────────────────────────────────────────────────

func TestErrUnauthorized_Has401(t *testing.T) {
	assert.Equal(t, http.StatusUnauthorized, apperrors.ErrUnauthorized.HTTPStatus)
}

func TestErrForbidden_Has403(t *testing.T) {
	assert.Equal(t, http.StatusForbidden, apperrors.ErrForbidden.HTTPStatus)
}

func TestErrNotFound_Has404(t *testing.T) {
	assert.Equal(t, http.StatusNotFound, apperrors.ErrNotFound.HTTPStatus)
}

func TestErrRateLimited_Has429(t *testing.T) {
	assert.Equal(t, http.StatusTooManyRequests, apperrors.ErrRateLimited.HTTPStatus)
}

func TestErrQuotaExhausted_Has402(t *testing.T) {
	assert.Equal(t, http.StatusPaymentRequired, apperrors.ErrQuotaExhausted.HTTPStatus)
}

func TestErrBodyTooLarge_Has413(t *testing.T) {
	assert.Equal(t, http.StatusRequestEntityTooLarge, apperrors.ErrBodyTooLarge.HTTPStatus)
}

func TestErrMissingSecurityHeader_Has400(t *testing.T) {
	assert.Equal(t, http.StatusBadRequest, apperrors.ErrMissingSecurityHeader.HTTPStatus)
}

func TestErrReplayDetected_Has401(t *testing.T) {
	assert.Equal(t, http.StatusUnauthorized, apperrors.ErrReplayDetected.HTTPStatus)
}

func TestErrTimestampInvalid_Has401(t *testing.T) {
	assert.Equal(t, http.StatusUnauthorized, apperrors.ErrTimestampInvalid.HTTPStatus)
}

// ── Wrap preserves all fields ─────────────────────────────────────────────────

func TestWrap_SetsAllFields(t *testing.T) {
	cause := errors.New("cause")
	err := apperrors.Wrap(http.StatusBadGateway, "GATEWAY_ERR", "gateway failed", cause)
	assert.Equal(t, http.StatusBadGateway, err.HTTPStatus)
	assert.Equal(t, "GATEWAY_ERR", err.Code)
	assert.Equal(t, "gateway failed", err.Message)
	assert.Equal(t, cause, err.Internal)
}

func TestNew_HasNilInternal(t *testing.T) {
	err := apperrors.New(http.StatusOK, "OK", "ok")
	require.NotNil(t, err)
	assert.Nil(t, err.Internal)
}
