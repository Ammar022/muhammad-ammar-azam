package domain

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"

	apperrors "github.com/Ammar022/secure-ai-chat-backend/internal/shared/errors"
)

type ChatRepository interface {
	Create(ctx context.Context, msg *ChatMessage) (*ChatMessage, error)
	FindByID(ctx context.Context, id uuid.UUID) (*ChatMessage, error)
	ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*ChatMessage, int64, error)
}

type QuotaRepository interface {
	GetOrCreateForMonth(ctx context.Context, tx *sqlx.Tx, userID uuid.UUID, month string) (*QuotaUsage, error)
	IncrementFreeUsage(ctx context.Context, tx *sqlx.Tx, id uuid.UUID) error
}

type SubscriptionForQuota struct {
	ID           uuid.UUID
	Tier         string // "basic" | "pro" | "enterprise"
	MessagesUsed int
	MaxMessages  int // -1 = unlimited
	IsActive     bool
}

type SubscriptionQuotaRepository interface {
	FindActiveForUserOrderedByCreatedDesc(ctx context.Context, tx *sqlx.Tx, userID uuid.UUID) ([]*SubscriptionForQuota, error)
	DeductMessage(ctx context.Context, tx *sqlx.Tx, subscriptionID uuid.UUID) error
}

type MockAIResponse struct {
	Answer           string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	LatencyMs        int64
}

func simulateOpenAI(question string, minLatencyMs, maxLatencyMs int) MockAIResponse {
	start := time.Now()

	// Simulate network + model inference latency
	latency := time.Duration(minLatencyMs+rand.Intn(maxLatencyMs-minLatencyMs+1)) * time.Millisecond
	time.Sleep(latency)

	// Approximate token counts (rough heuristic: 1 token ≈ 4 characters)
	promptTokens := max(1, len(question)/4)
	answer := fmt.Sprintf(
		"This is a simulated AI response to your question: \"%s\". "+
			"In a production system this would be the response from the OpenAI Chat Completions API. "+
			"The system is working correctly — your question was received, quota was validated, "+
			"and this response has been stored in the database with full token accounting.",
		question,
	)
	completionTokens := max(1, len(answer)/4)

	return MockAIResponse{
		Answer:           answer,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
		LatencyMs:        time.Since(start).Milliseconds(),
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type ChatService struct {
	db           *sqlx.DB
	chatRepo     ChatRepository
	quotaRepo    QuotaRepository
	subQuotaRepo SubscriptionQuotaRepository
	policy       *QuotaPolicy
	minLatencyMs int
	maxLatencyMs int
}

func NewChatService(
	db *sqlx.DB,
	chatRepo ChatRepository,
	quotaRepo QuotaRepository,
	subQuotaRepo SubscriptionQuotaRepository,
	minLatencyMs, maxLatencyMs int,
) *ChatService {
	return &ChatService{
		db:           db,
		chatRepo:     chatRepo,
		quotaRepo:    quotaRepo,
		subQuotaRepo: subQuotaRepo,
		policy:       NewQuotaPolicy(),
		minLatencyMs: minLatencyMs,
		maxLatencyMs: maxLatencyMs,
	}
}

func (s *ChatService) SendMessage(
	ctx context.Context,
	requestingUserID uuid.UUID,
	question string,
	ipAddress string,
	requestID string,
) (*ChatMessage, error) {
	if err := s.policy.CanSendMessage(requestingUserID, requestingUserID); err != nil {
		return nil, err
	}

	var chargedSubscriptionID *uuid.UUID

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("chat service: begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	month := CurrentMonth()
	quota, err := s.quotaRepo.GetOrCreateForMonth(ctx, tx, requestingUserID, month)
	if err != nil {
		return nil, fmt.Errorf("chat service: get quota: %w", err)
	}

	if quota.FreeMessagesUsed < FreeMessagesPerMonth {
		if err = s.quotaRepo.IncrementFreeUsage(ctx, tx, quota.ID); err != nil {
			return nil, fmt.Errorf("chat service: increment free usage: %w", err)
		}
		log.Ctx(ctx).Debug().
			Str("user_id", requestingUserID.String()).
			Int("free_used", quota.FreeMessagesUsed+1).
			Msg("chat: charged free quota")
	} else {
		subID, subErr := s.chargeFromSubscriptions(ctx, tx, requestingUserID)
		if subErr != nil {
			err = subErr
			return nil, err
		}
		chargedSubscriptionID = subID
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("chat service: commit transaction: %w", err)
	}

	aiResp := simulateOpenAI(question, s.minLatencyMs, s.maxLatencyMs)

	msg := &ChatMessage{
		ID:               uuid.New(),
		UserID:           requestingUserID,
		SubscriptionID:   chargedSubscriptionID,
		Question:         question,
		Answer:           aiResp.Answer,
		PromptTokens:     aiResp.PromptTokens,
		CompletionTokens: aiResp.CompletionTokens,
		TotalTokens:      aiResp.TotalTokens,
		ResponseTimeMs:   aiResp.LatencyMs,
		IPAddress:        ipAddress,
		RequestID:        requestID,
	}

	saved, err := s.chatRepo.Create(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("chat service: save message: %w", err)
	}

	return saved, nil
}

func (s *ChatService) chargeFromSubscriptions(ctx context.Context, tx *sqlx.Tx, userID uuid.UUID) (*uuid.UUID, error) {
	subs, err := s.subQuotaRepo.FindActiveForUserOrderedByCreatedDesc(ctx, tx, userID)
	if err != nil {
		return nil, fmt.Errorf("chat service: find subscriptions: %w", err)
	}
	for _, sub := range subs {
		if sub.Tier == "enterprise" || sub.MaxMessages == -1 || sub.MessagesUsed < sub.MaxMessages {
			if err = s.subQuotaRepo.DeductMessage(ctx, tx, sub.ID); err != nil {
				return nil, fmt.Errorf("chat service: deduct message: %w", err)
			}
			id := sub.ID
			return &id, nil
		}
	}
	return nil, apperrors.ErrNoActiveSubscription
}

// GetMessage returns a single chat message, enforcing ownership via domain policy
func (s *ChatService) GetMessage(ctx context.Context, requestingUserID, messageID uuid.UUID) (*ChatMessage, error) {
	msg, err := s.chatRepo.FindByID(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, apperrors.ErrNotFound
	}
	if err := s.policy.CanViewMessage(requestingUserID, msg.UserID); err != nil {
		return nil, err
	}
	return msg, nil
}

// ListMessages returns the authenticated user's chat history with pagination
func (s *ChatService) ListMessages(ctx context.Context, userID uuid.UUID, page, perPage int) ([]*ChatMessage, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage
	return s.chatRepo.ListByUserID(ctx, userID, perPage, offset)
}
