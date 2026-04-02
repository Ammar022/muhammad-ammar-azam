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

// ── Repository interfaces ─────────────────────────────────────────────────────
// Defined here (owned by the domain) so the domain layer has zero dependency
// on any infrastructure package.  The repository layer implements these.

// ChatRepository persists and retrieves chat messages.
type ChatRepository interface {
	Create(ctx context.Context, msg *ChatMessage) (*ChatMessage, error)
	FindByID(ctx context.Context, id uuid.UUID) (*ChatMessage, error)
	ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*ChatMessage, int64, error)
}

// QuotaRepository manages monthly quota usage records.
type QuotaRepository interface {
	// GetOrCreateForMonth fetches the quota record for the user/month, creating
	// it (with 0 used) if it does not yet exist.  Must be called inside a
	// transaction to be safe under concurrent requests.
	GetOrCreateForMonth(ctx context.Context, tx *sqlx.Tx, userID uuid.UUID, month string) (*QuotaUsage, error)
	// IncrementFreeUsage atomically increments free_messages_used within a tx.
	IncrementFreeUsage(ctx context.Context, tx *sqlx.Tx, id uuid.UUID) error
}

// SubscriptionForQuota is a minimal projection of a subscription used by the
// quota engine.  It avoids a circular import between the chat and subscription
// packages while still expressing the data the quota engine needs.
type SubscriptionForQuota struct {
	ID           uuid.UUID
	Tier         string // "basic" | "pro" | "enterprise"
	MessagesUsed int
	MaxMessages  int // -1 = unlimited
	IsActive     bool
}

// SubscriptionQuotaRepository is the quota-side contract that the subscription
// package must satisfy (implemented in subscription/repository).
type SubscriptionQuotaRepository interface {
	// FindActiveForUserOrderedByCreatedDesc returns active subscriptions for the
	// user with remaining capacity, newest first.
	FindActiveForUserOrderedByCreatedDesc(ctx context.Context, tx *sqlx.Tx, userID uuid.UUID) ([]*SubscriptionForQuota, error)
	// DeductMessage atomically increments messages_used for a subscription
	// within a transaction.
	DeductMessage(ctx context.Context, tx *sqlx.Tx, subscriptionID uuid.UUID) error
}

// ── AI response simulation ────────────────────────────────────────────────────

// MockAIResponse is the structured result of the simulated OpenAI API call.
type MockAIResponse struct {
	Answer           string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	LatencyMs        int64
}

// simulateOpenAI generates a deterministic-looking mocked OpenAI response with
// a configurable random latency to simulate real API round-trip times.
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

// ── ChatService ───────────────────────────────────────────────────────────────

// ChatService orchestrates the full message-send workflow:
//  1. Determine quota source (free tier vs. subscription)
//  2. Atomically deduct quota in a transaction
//  3. Call the mocked AI
//  4. Persist the chat message
//
// It depends on repository interfaces, not concrete types, so it is fully
// testable without a real database.
type ChatService struct {
	db           *sqlx.DB
	chatRepo     ChatRepository
	quotaRepo    QuotaRepository
	subQuotaRepo SubscriptionQuotaRepository
	policy       *QuotaPolicy
	minLatencyMs int
	maxLatencyMs int
}

// NewChatService creates a ChatService with all required dependencies.
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

// SendMessage is the primary use-case method.  It:
//  1. Enforces domain policy (ownership check).
//  2. Deducts quota atomically (serializable transaction with SELECT FOR UPDATE).
//  3. Calls the mocked OpenAI service.
//  4. Persists the resulting ChatMessage.
func (s *ChatService) SendMessage(
	ctx context.Context,
	requestingUserID uuid.UUID,
	question string,
	ipAddress string,
	requestID string,
) (*ChatMessage, error) {
	// ── Domain policy check ──────────────────────────────────────────────────
	// (In a single-user context the requesting user IS the resource owner, but
	//  this check becomes meaningful if an admin acts on behalf of a user.)
	if err := s.policy.CanSendMessage(requestingUserID, requestingUserID); err != nil {
		return nil, err
	}

	// ── Atomic quota deduction (transaction) ─────────────────────────────────
	// We use READ COMMITTED with explicit locking (SELECT … FOR UPDATE) to
	// prevent two concurrent requests from both seeing 0 usage and both
	// succeeding against a free slot that only one of them should get.
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
		// ── Case 1: free quota available ────────────────────────────────────
		if err = s.quotaRepo.IncrementFreeUsage(ctx, tx, quota.ID); err != nil {
			return nil, fmt.Errorf("chat service: increment free usage: %w", err)
		}
		log.Ctx(ctx).Debug().
			Str("user_id", requestingUserID.String()).
			Int("free_used", quota.FreeMessagesUsed+1).
			Msg("chat: charged free quota")
	} else {
		// ── Case 2: try subscription bundles (newest first) ──────────────────
		subs, err := s.subQuotaRepo.FindActiveForUserOrderedByCreatedDesc(ctx, tx, requestingUserID)
		if err != nil {
			return nil, fmt.Errorf("chat service: find subscriptions: %w", err)
		}

		charged := false
		for _, sub := range subs {
			// Enterprise tier: unlimited — always charge (no capacity check)
			if sub.Tier == "enterprise" || sub.MaxMessages == -1 {
				if err = s.subQuotaRepo.DeductMessage(ctx, tx, sub.ID); err != nil {
					return nil, fmt.Errorf("chat service: deduct message: %w", err)
				}
				id := sub.ID
				chargedSubscriptionID = &id
				charged = true
				break
			}
			// Other tiers: check remaining capacity
			if sub.MessagesUsed < sub.MaxMessages {
				if err = s.subQuotaRepo.DeductMessage(ctx, tx, sub.ID); err != nil {
					return nil, fmt.Errorf("chat service: deduct message: %w", err)
				}
				id := sub.ID
				chargedSubscriptionID = &id
				charged = true
				break
			}
		}

		if !charged {
			err = apperrors.ErrNoActiveSubscription
			return nil, err
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("chat service: commit transaction: %w", err)
	}

	// ── Call the mocked AI (after quota committed) ────────────────────────────
	aiResp := simulateOpenAI(question, s.minLatencyMs, s.maxLatencyMs)

	// ── Persist the chat message ──────────────────────────────────────────────
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

// GetMessage returns a single chat message, enforcing ownership via domain policy.
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

// ListMessages returns the authenticated user's chat history with pagination.
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
