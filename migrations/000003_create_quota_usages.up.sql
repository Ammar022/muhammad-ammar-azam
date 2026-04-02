-- Migration: 000003_create_quota_usages
-- Tracks monthly free-tier usage per user.
-- One row per (user_id, month); reset implicitly by inserting a new row each month.

CREATE TABLE IF NOT EXISTS quota_usages (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    month               TEXT        NOT NULL,   -- 'YYYY-MM', e.g. '2025-04'
    free_messages_used  INTEGER     NOT NULL DEFAULT 0
                                    CHECK (free_messages_used >= 0),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One record per user per calendar month
    UNIQUE (user_id, month)
);

CREATE INDEX IF NOT EXISTS idx_quota_usages_user_month ON quota_usages (user_id, month);
