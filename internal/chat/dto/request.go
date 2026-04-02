// Package dto defines Data Transfer Objects for the chat module.
// DTOs live at the boundary between the HTTP layer and the domain; they
// carry only the fields the transport layer needs and are validated before
// being passed to the domain service.
package dto

// SendMessageRequest is the body accepted by POST /api/v1/chat/messages.
type SendMessageRequest struct {
	// Question is the user's prompt.  Required, 1–4000 characters.
	// The validator rejects blank strings and anything exceeding the limit.
	Question string `json:"question" validate:"required,min=1,max=4000" example:"What is quantum computing and how does it work?"`
}

// ListMessagesQuery contains the validated query parameters for pagination.
type ListMessagesQuery struct {
	// Page is 1-indexed.  Defaults to 1.
	Page int `schema:"page" validate:"min=1"`
	// PerPage controls page size.  Capped at 100 by the service layer.
	PerPage int `schema:"per_page" validate:"min=1,max=100"`
}
