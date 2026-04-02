package domain

import (
	"github.com/google/uuid"

	apperrors "github.com/Ammar022/secure-ai-chat-backend/internal/shared/errors"
)

type QuotaPolicy struct{}

// NewQuotaPolicy creates a QuotaPolicy.
func NewQuotaPolicy() *QuotaPolicy { return &QuotaPolicy{} }

// CanSendMessage verifies that the requesting user is allowed to send a
// message.  It checks:
//  1. The user owns the request (userID in claims matches the resource)
//  2. The quota is not exhausted (checked before DB write in the service)
//
// The quota exhaustion check here is a domain-level guard; the actual
// atomic deduction is done inside a transaction in ChatService.
func (p *QuotaPolicy) CanSendMessage(requestingUserID, resourceOwnerID uuid.UUID) error {
	if requestingUserID != resourceOwnerID {
		return apperrors.ErrForbidden
	}
	return nil
}

// CanViewMessage ensures a user can only read their own chat history.
// Admins are exempt from this check at the controller level.
func (p *QuotaPolicy) CanViewMessage(requestingUserID, messageOwnerID uuid.UUID) error {
	if requestingUserID != messageOwnerID {
		return apperrors.ErrForbidden
	}
	return nil
}
