// Package domain contains the Chat aggregate — messages, quota tracking,
// and the invariants that govern message creation and quota deduction.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// ChatMessage is the aggregate root for a single AI conversation turn.
// It captures the full request/response lifecycle including token accounting
// and the quota source that was charged.
type ChatMessage struct {
	// ID is the internal UUID primary key.
	ID uuid.UUID `db:"id"`
	// UserID references the internal users.id (not the Auth0 subject).
	UserID uuid.UUID `db:"user_id"`
	// SubscriptionID is the subscription charged for this message.
	// NULL means the user's free monthly quota was used.
	SubscriptionID *uuid.UUID `db:"subscription_id"`
	// Question is the user's raw input.  Stored sanitized.
	Question string `db:"question"`
	// Answer is the mocked AI response.
	Answer string `db:"answer"`
	// Token usage breakdown (mirrors the OpenAI API response shape).
	PromptTokens     int `db:"prompt_tokens"`
	CompletionTokens int `db:"completion_tokens"`
	TotalTokens      int `db:"total_tokens"`
	// ResponseTimeMs is the simulated AI latency in milliseconds.
	ResponseTimeMs int64 `db:"response_time_ms"`
	// IPAddress is the client's source IP, stored for audit purposes.
	IPAddress string `db:"ip_address"`
	// RequestID correlates this record with the request log entry.
	RequestID string `db:"request_id"`
	// CreatedAt is set by the database on INSERT.
	CreatedAt time.Time `db:"created_at"`
}

// QuotaUsage tracks how many free messages a user has consumed in a
// given calendar month.  The month field uses "YYYY-MM" format so that
// SQL range queries are straightforward.
type QuotaUsage struct {
	ID     uuid.UUID `db:"id"`
	UserID uuid.UUID `db:"user_id"`
	// Month is the calendar month this record covers, e.g. "2025-04".
	Month string `db:"month"`
	// FreeMessagesUsed is the count of messages charged to the free tier.
	FreeMessagesUsed int       `db:"free_messages_used"`
	CreatedAt        time.Time `db:"created_at"`
	UpdatedAt        time.Time `db:"updated_at"`
}

// FreeMessagesPerMonth is the number of messages every user gets at no cost
// per calendar month before a paid subscription is required.
const FreeMessagesPerMonth = 3

// CurrentMonth returns the "YYYY-MM" string for the current UTC month.
func CurrentMonth() string {
	return time.Now().UTC().Format("2006-01")
}
