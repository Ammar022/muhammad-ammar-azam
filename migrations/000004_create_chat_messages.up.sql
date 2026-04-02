-- Migration: 000004_create_chat_messages
-- Stores every AI conversation turn with full token accounting and audit fields.
-- subscription_id is nullable: NULL means the free monthly quota was charged.

CREATE TABLE IF NOT EXISTS chat_messages (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subscription_id   UUID        REFERENCES subscriptions(id) ON DELETE SET NULL,
    question          TEXT        NOT NULL,
    answer            TEXT        NOT NULL,
    prompt_tokens     INTEGER     NOT NULL DEFAULT 0,
    completion_tokens INTEGER     NOT NULL DEFAULT 0,
    total_tokens      INTEGER     NOT NULL DEFAULT 0,
    response_time_ms  BIGINT      NOT NULL DEFAULT 0,
    ip_address        TEXT        NOT NULL DEFAULT '',
    request_id        TEXT        NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- User's own message history (pagination queries)
CREATE INDEX IF NOT EXISTS idx_chat_messages_user_id ON chat_messages (user_id, created_at DESC);
-- Admin analytics: messages per subscription
CREATE INDEX IF NOT EXISTS idx_chat_messages_subscription_id ON chat_messages (subscription_id);
