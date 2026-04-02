// Package response provides helpers for writing consistent, structured JSON
// responses.  All API responses use the same envelope format so clients can
// rely on a predictable shape regardless of endpoint.
package response

import (
	"encoding/json"
	"net/http"

	apperrors "github.com/Ammar022/secure-ai-chat-backend/internal/shared/errors"
)

// Envelope is the standard API response wrapper.
//
//	Success:  { "success": true,  "data": <payload>,  "meta": <pagination> }
//	Error:    { "success": false, "error": { "code": "...", "message": "..." } }
type Envelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorBody  `json:"error,omitempty"`
	Meta    interface{} `json:"meta,omitempty"`
}

// ErrorBody is the structured error payload returned on failures.
type ErrorBody struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"` // optional field-level errors
}

// PaginationMeta carries standard pagination metadata.
type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

// JSON serialises payload to JSON and writes it with the given status code.
// It sets Content-Type: application/json automatically.
func JSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// If we fail here the headers are already sent; log it and move on.
		http.Error(w, `{"success":false,"error":{"code":"ENCODE_ERROR","message":"response encoding failed"}}`, http.StatusInternalServerError)
	}
}

// Success writes a 200 OK JSON response with the given data.
func Success(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusOK, Envelope{Success: true, Data: data})
}

// Created writes a 201 Created JSON response with the given data.
func Created(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusCreated, Envelope{Success: true, Data: data})
}

// NoContent writes a 204 No Content response.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Paginated writes a 200 OK JSON response with data and pagination metadata.
func Paginated(w http.ResponseWriter, data interface{}, meta PaginationMeta) {
	JSON(w, http.StatusOK, Envelope{Success: true, Data: data, Meta: meta})
}

// Error writes a structured error JSON response derived from an AppError.
// Internal error details (e.g. database errors) are NEVER included in the
// response body – they are only logged server-side.
func Error(w http.ResponseWriter, err error) {
	ae := apperrors.ToAppError(err)
	JSON(w, ae.HTTPStatus, Envelope{
		Success: false,
		Error: &ErrorBody{
			Code:    ae.Code,
			Message: ae.Message,
		},
	})
}

// DecodeJSON decodes the request body into dst.
// It disallows unknown fields to prevent mass-assignment and limits the body
// to the same size enforced by the RequestSizeLimit middleware.
func DecodeJSON(r *http.Request, dst interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// ErrorWithDetails writes a structured error response that includes optional
// field-level detail (e.g. validation failures).
func ErrorWithDetails(w http.ResponseWriter, err error, details interface{}) {
	ae := apperrors.ToAppError(err)
	JSON(w, ae.HTTPStatus, Envelope{
		Success: false,
		Error: &ErrorBody{
			Code:    ae.Code,
			Message: ae.Message,
			Details: details,
		},
	})
}
