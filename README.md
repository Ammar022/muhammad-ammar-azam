# Muhammad Ammar Azam — Secure AI Chat Backend

[![Go Report Card](https://goreportcard.com/badge/github.com/Ammar022/muhammad-ammar-azam)](https://goreportcard.com/report/github.com/Ammar022/muhammad-ammar-azam)

A production-grade, secure REST API built with **Go**, implementing an **AI Chat Module** and a **Subscription Bundle Module**. The architecture follows **Domain-Driven Design (DDD)** and **Clean Architecture** principles.

> **Language note:** This submission is in **Go** ; as discussed. All functional, architectural, and security requirements from the brief are fully satisfied.

**Authentication**: Auth0 (OIDC/OAuth2) — RS256 JWT validation via JWKS.

---

## Table of Contents

- [Architecture Decisions](#architecture-decisions)
- [Security Model](#security-model)
- [Features](#features)
- [Project Structure](#project-structure)
- [Quick Start](#quick-start)
- [Environment Variables](#environment-variables)
- [API Reference](#api-reference)
- [Testing](#testing)

---

## Architecture Decisions

### 1. Domain-Driven Design with Clean Architecture

Each business module (`chat`, `subscription`, `user`, `admin`) is fully self-contained with strict layer separation:

```
domain/entity.go     ← Pure Go structs, no framework imports
domain/policy.go     ← Authorization rules as pure functions
domain/service.go    ← Orchestration (quota, AI call, persistence)
repository/          ← Interface defined in domain; Postgres impl separate
dto/                 ← Input shapes for HTTP layer only
controller/http.go   ← Thin HTTP adapter; delegates to service
```

Business logic has **zero dependency** on chi, HTTP, or any transport concern. The domain layer is tested in pure unit tests with no database or server setup.

### 2. Atomic Quota Deduction

The free quota and subscription bundle quota are deducted using `SELECT ... FOR UPDATE` inside a `SERIALIZABLE` transaction. This prevents double-spending under concurrent requests — a critical correctness constraint for a billing system.

### 3. Anti-Replay via Nonce + Timestamp

Rather than session-bound tokens or proof-of-possession (which require client-side key management), we use a stateless anti-replay scheme:
- `X-Request-Timestamp` — Unix epoch; rejected if outside ±`SECURITY_ANTI_REPLAY_WINDOW_SEC` of server time
- `X-Nonce` — unique string per request; stored in an in-memory LRU cache; replay returns 401

This satisfies the requirement that *"possession of an access token alone must not be sufficient to access APIs"* without requiring stateful sessions.

### 4. Auth0 as External Provider (no custom auth)

Auth0 handles all credential management, MFA, social login, and token signing. The API trusts Auth0's RS256-signed JWTs validated against the JWKS endpoint — standard OIDC. No password hashing, no session store, no custom token issuance in this codebase.

### 5. Role Propagation: JWT → DB → Context

Roles flow in one direction:
1. Auth0 Action injects roles into a custom JWT claim (`AUTH0_ROLES_CLAIM`)
2. JWT middleware extracts and normalises roles (lowercase)
3. `UserSync` middleware upserts the user in the local DB, syncing the role
4. DB role can promote a user even before their token is refreshed (fallback escalation)

### 6. Circular Import Prevention

`Role` type and `UserSync` middleware live in `user/domain` and `shared/middleware` respectively — not in `shared/auth` — to avoid the circular dependency `auth ↔ user/domain`.

### 7. Subscription Renewal as a Background Job

A goroutine runs every `RENEWAL_INTERVAL_MINUTES` (default 60) to process expiring subscriptions. Payment is simulated with a 30% failure rate. Failed payments mark subscriptions inactive; successful ones extend the billing window and reset message counts.

### 8. Test Strategy

- **Unit tests** — pure domain logic; no DB, no HTTP, no Auth0
- **Integration tests** — real HTTP stack (`httptest`) with a mock JWKS server serving genuine RS256-signed JWTs; no DB required
- Auth0 is **mocked at the JWKS level**, not bypassed — the same JWT validation code path runs in tests as in production

---

## Security Model

### Authentication Flow

```
Client obtains JWT from Auth0 (dashboard test tab, password grant, or social login)

API request → Authorization: Bearer <token>
  → Extract token from header
  → Fetch Auth0 JWKS (cached, auto-refreshed every 15 min)
  → Validate RS256 signature
  → Verify issuer (AUTH0_DOMAIN) + audience (AUTH0_AUDIENCE) + expiry
  → Extract subject, email, roles from claims (normalised to lowercase)
  → UserSync middleware — upsert local user record, populate InternalUserID
  → Claims injected into request context
```

### Authorization Layers

1. **JWT middleware** — every `/api/v1` request must carry a valid, non-expired token
2. **RBAC middleware** — `RequireRole(admin)` guards all `/api/v1/admin/*` routes
3. **Domain policy** — pure functions enforce ownership (users can only read/write their own resources)

### Anti-Replay

Every authenticated request must include:
- `X-Request-Timestamp` — Unix epoch; rejected outside ±`SECURITY_ANTI_REPLAY_WINDOW_SEC` seconds (default ±300s)
- `X-Nonce` — unique string per request; cached server-side; duplicate returns 401

Missing headers → **400**. Valid nonce reused → **401**.

### Rate Limiting

| Scope | Default |
|---|---|
| Global IP limit (pre-auth) | 120 req/min |
| Per-user limit (post-auth) | 60 req/min |
| Chat endpoint | 20 req/min |

### Other Protections

- Secure HTTP headers: CSP, HSTS, X-Frame-Options, X-Content-Type-Options, Referrer-Policy, Permissions-Policy
- Restricted CORS (configurable origins; credentials not exposed)
- Request body size limit (default 1 MB)
- `Content-Type: application/json` enforced on all mutating endpoints
- Global request timeout (returns 504 if exceeded)
- XSS sanitisation via `bluemonday.StrictPolicy()` on all user input
- Unknown JSON fields rejected (`DisallowUnknownFields`)

---

## Features

### AI Chat Module
- Authenticated chat endpoint backed by a **mocked OpenAI response**
- Full token accounting (prompt / completion / total)
- **Monthly free quota** (3 messages/user/month) with atomic deduction
- Falls back to **subscription bundles** when free quota is exhausted

### Subscription Bundle Module
- Three tiers: **Basic** (10 msg), **Pro** (100 msg), **Enterprise** (unlimited)
- Monthly and yearly billing cycles (yearly = 20% discount)
- **Auto-renew** with simulated payment processing (30% failure rate)
- **Cancellation** preserves usage data and grants access until end of billing period
- Background renewal job (hourly)

### Observability
- Structured JSON logging with zerolog — request ID, user ID, latency on every request
- **Health endpoint** (`GET /health`) — DB connectivity check, version, uptime
- **Admin metrics endpoint** (`GET /api/v1/admin/metrics`) — usage statistics, admin-only

---

## Project Structure

```
muhammad-ammar-azam/
├── cmd/api/main.go                     # Entry point — composition root
├── docs/                               # Submission PDF
├── internal/
│   ├── shared/
│   │   ├── config/        config.go    # Viper-based env config
│   │   ├── database/      database.go  # sqlx wrapper, migrations
│   │   ├── logger/        logger.go    # zerolog setup
│   │   ├── errors/        errors.go    # Typed AppError with HTTP codes
│   │   ├── response/      response.go  # JSON envelope helpers
│   │   ├── auth/
│   │   │   ├── claims.go              # JWT claims struct + context helpers
│   │   │   └── middleware.go          # Auth0 RS256 JWKS validator + RequireRole
│   │   └── middleware/
│   │       ├── requestid.go           # UUID request ID propagation
│   │       ├── logger.go              # Structured request/response logging
│   │       ├── security.go            # Secure headers, size limit, JSON enforcement
│   │       ├── antireplay.go          # Nonce + timestamp replay prevention
│   │       ├── ratelimit.go           # Token bucket rate limiting (IP + user)
│   │       ├── timeout.go             # Context-based request timeout
│   │       └── usersync.go            # OAuth subject → local user upsert
│   ├── auth/
│   │   └── controller/http.go         # Admin SetRole endpoint
│   ├── user/
│   │   ├── domain/entity.go           # User aggregate root + Role type
│   │   └── repository/postgres.go     # PostgreSQL implementation
│   ├── chat/
│   │   ├── domain/
│   │   │   ├── entity.go              # ChatMessage + QuotaUsage entities
│   │   │   ├── policy.go              # QuotaPolicy — pure domain rules
│   │   │   └── service.go             # ChatService — orchestrates quota + AI
│   │   ├── repository/postgres.go     # Chat + quota PostgreSQL repos
│   │   ├── dto/request.go             # Input DTOs with validation tags
│   │   └── controller/http.go         # chi HTTP handlers
│   ├── subscription/
│   │   ├── domain/
│   │   │   ├── entity.go              # Subscription aggregate root
│   │   │   ├── policy.go              # SubscriptionPolicy
│   │   │   └── service.go             # SubscriptionService + renewal job
│   │   ├── repository/postgres.go     # Subscription PostgreSQL repo
│   │   ├── dto/request.go             # Input DTOs
│   │   └── controller/http.go         # chi HTTP handlers
│   └── admin/
│       └── controller/http.go         # Admin metrics + user management
├── migrations/
│   ├── 000001_create_users.*
│   ├── 000002_create_subscriptions.*
│   ├── 000003_create_quota_usages.*
│   └── 000004_create_chat_messages.*
├── tests/
│   ├── unit/                          # Domain logic, quota math, subscription lifecycle
│   └── integration/                   # Middleware, auth (mocked JWKS), rate limiting
├── Dockerfile
├── docker-compose.yml
├── Makefile
└── .env.example
```

---

## Quick Start

### Prerequisites

- Go 1.22+
- Docker & Docker Compose
- An [Auth0](https://auth0.com) account with an API and Application configured

### 1. Clone and configure

```bash
git clone https://github.com/Ammar022/muhammad-ammar-azam
cd muhammad-ammar-azam
cp .env.example .env
# Edit .env — set AUTH0_DOMAIN, AUTH0_AUDIENCE, AUTH0_ROLES_CLAIM
```

### 2. Start with Docker Compose

```bash
# Starts PostgreSQL + the API
make docker-up

# Or just start the DB and run the API locally (faster iteration):
make docker-db
make run
```

### 3. Obtain an access token

#### Option A — Email / password (Resource Owner Password Grant)

Requires the **Password** grant type enabled on your Auth0 Application and a **Default Directory** set on your tenant.

```bash
curl -X POST https://<AUTH0_DOMAIN>/oauth/token \
  -H "Content-Type: application/json" \
  -d '{
    "client_id":     "<YOUR_CLIENT_ID>",
    "client_secret": "<YOUR_CLIENT_SECRET>",
    "audience":      "<AUTH0_AUDIENCE>",
    "grant_type":    "password",
    "username":      "<USER_EMAIL>",
    "password":      "<USER_PASSWORD>",
    "scope":         "openid profile email"
  }'
```

#### Option B — Google OAuth (Authorization Code flow)

1. Enable Google social connection in Auth0 Dashboard → Authentication → Social → Google.
2. Add `http://localhost:3000/callback` to **Allowed Callback URLs** on your Application.
3. Open this URL in a browser and log in with Google:

```
https://<AUTH0_DOMAIN>/authorize
  ?response_type=code
  &client_id=<YOUR_CLIENT_ID>
  &redirect_uri=http://localhost:3000/callback
  &audience=<AUTH0_AUDIENCE>
  &scope=openid%20profile%20email
  &connection=google-oauth2
```

4. After login the browser redirects to `localhost:3000/callback?code=<AUTH_CODE>`. Copy the code.
5. Exchange the code for tokens:

```bash
curl -X POST https://<AUTH0_DOMAIN>/oauth/token \
  -H "Content-Type: application/json" \
  -d '{
    "grant_type":    "authorization_code",
    "client_id":     "<YOUR_CLIENT_ID>",
    "client_secret": "<YOUR_CLIENT_SECRET>",
    "code":          "<AUTH_CODE>",
    "redirect_uri":  "http://localhost:3000/callback"
  }'
```

#### Using the token

```bash
export TOKEN="<ACCESS_TOKEN_FROM_ABOVE>"

# All API requests require these three headers:
curl http://localhost:8080/api/v1/subscriptions \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Nonce: $(uuidgen)" \
  -H "X-Request-Timestamp: $(date +%s)"
```

> **Note:** Tokens expire after 24 hours. Re-run the token request to get a fresh one.


### 5. Run tests

```bash
make test              # all tests (unit + integration)
make test-unit         # unit tests only (no external deps)
make test-integration  # integration tests (httptest, no DB required)
```

---

## Environment Variables

See [`.env.example`](./.env.example) for the full list with descriptions.

| Variable | Required | Description |
|----------|----------|-------------|
| `APP_PORT` | — | HTTP port (default: 8080) |
| `DB_HOST` | ✓ | PostgreSQL host |
| `DB_NAME` | ✓ | Database name |
| `AUTH0_DOMAIN` | ✓ | Auth0 tenant domain (e.g. `dev-xxx.us.auth0.com`) |
| `AUTH0_AUDIENCE` | ✓ | Auth0 API identifier (e.g. `https://chat-api`) |
| `AUTH0_ROLES_CLAIM` | — | Custom namespace for roles claim (default: `https://api.yourdomain.com/roles`) |
| `CORS_ALLOWED_ORIGINS` | — | Comma-separated allowed origins (default: `http://localhost:3000`) |
| `RATE_LIMIT_IP_RPM` | — | IP rate limit req/min (default: 100) |
| `RATE_LIMIT_USER_RPM` | — | Per-user rate limit req/min (default: 50) |
| `AI_LATENCY_MIN_MS` | — | Mock AI min latency ms (default: 500) |
| `AI_LATENCY_MAX_MS` | — | Mock AI max latency ms (default: 2000) |
| `RENEWAL_INTERVAL_MINUTES` | — | Subscription renewal job interval (default: 60) |

---

## Testing

```bash
# Unit tests — no external dependencies
go test ./tests/unit/... -v

# Integration tests — uses httptest, no DB required
go test ./tests/integration/... -v

# All tests with race detector
go test -race ./...

# Coverage report
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```
