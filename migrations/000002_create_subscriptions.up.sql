-- Migration: 000002_create_subscriptions
-- Stores subscription bundles.  A user may have multiple active bundles
-- simultaneously (different tiers, different billing cycles).

CREATE TABLE IF NOT EXISTS subscriptions (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tier          TEXT        NOT NULL CHECK (tier IN ('basic', 'pro', 'enterprise')),
    billing_cycle TEXT        NOT NULL CHECK (billing_cycle IN ('monthly', 'yearly')),
    auto_renew    BOOLEAN     NOT NULL DEFAULT TRUE,
    max_messages  INTEGER     NOT NULL,   -- -1 = unlimited (enterprise)
    messages_used INTEGER     NOT NULL DEFAULT 0,
    price         NUMERIC(10,2) NOT NULL, -- billed amount per cycle at creation time
    start_date    TIMESTAMPTZ NOT NULL,
    end_date      TIMESTAMPTZ NOT NULL,
    renewal_date  TIMESTAMPTZ NOT NULL,
    is_active     BOOLEAN     NOT NULL DEFAULT TRUE,
    cancelled_at  TIMESTAMPTZ,            -- NULL while active
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Quota check: find active subscriptions quickly for a user
CREATE INDEX IF NOT EXISTS idx_subscriptions_user_id        ON subscriptions (user_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_user_active    ON subscriptions (user_id, is_active, end_date);
-- Renewal job: find subscriptions due for renewal
CREATE INDEX IF NOT EXISTS idx_subscriptions_renewal        ON subscriptions (renewal_date) WHERE auto_renew = TRUE AND is_active = TRUE;
