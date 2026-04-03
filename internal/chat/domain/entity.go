package domain

import (
	"time"

	"github.com/google/uuid"
)

type ChatMessage struct {
	ID uuid.UUID `db:"id"`

	UserID         uuid.UUID  `db:"user_id"`
	SubscriptionID *uuid.UUID `db:"subscription_id"`

	Question         string `db:"question"`
	Answer           string `db:"answer"`
	PromptTokens     int    `db:"prompt_tokens"`
	CompletionTokens int    `db:"completion_tokens"`
	TotalTokens      int    `db:"total_tokens"`
	ResponseTimeMs   int64  `db:"response_time_ms"`
	IPAddress        string `db:"ip_address"`
	RequestID        string `db:"request_id"`

	CreatedAt time.Time `db:"created_at"`
}

type QuotaUsage struct {
	ID     uuid.UUID `db:"id"`
	UserID uuid.UUID `db:"user_id"`

	Month            string `db:"month"`
	FreeMessagesUsed int    `db:"free_messages_used"`

	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

const FreeMessagesPerMonth = 3

func CurrentMonth() string {
	return time.Now().UTC().Format("2006-01")
}
