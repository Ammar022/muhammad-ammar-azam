package domain

import (
	"github.com/google/uuid"

	apperrors "github.com/Ammar022/secure-ai-chat-backend/internal/shared/errors"
)

type QuotaPolicy struct{}

func NewQuotaPolicy() *QuotaPolicy { return &QuotaPolicy{} }

func (p *QuotaPolicy) CanSendMessage(requestingUserID, resourceOwnerID uuid.UUID) error {
	if requestingUserID != resourceOwnerID {
		return apperrors.ErrForbidden
	}
	return nil
}

func (p *QuotaPolicy) CanViewMessage(requestingUserID, messageOwnerID uuid.UUID) error {
	if requestingUserID != messageOwnerID {
		return apperrors.ErrForbidden
	}
	return nil
}
