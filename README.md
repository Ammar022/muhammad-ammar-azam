# Secure AI Chat Backend

[![Go Report Card](https://goreportcard.com/badge/github.com/Ammar022/secure-ai-chat-backend)](https://goreportcard.com/report/github.com/Ammar022/secure-ai-chat-backend)

A production-grade, secure REST API built with **Go**, implementing an **AI Chat Module** and a **Subscription Bundle Module**. The architecture follows **Domain-Driven Design (DDD)** and **Clean Architecture** principles.

**Authentication**: Auth0 (OIDC/OAuth2) — RS256 JWT validation via JWKS.

---

## Table of Contents

- [Features](#features)
- [Project Structure](#project-structure)
- [Quick Start](#quick-start)
- [Environment Variables](#environment-variables)
- [API Reference](#api-reference)
- [Security Model](#security-model)
- [Testing](#testing)

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

### Security
- **Auth0** — RS256 JWT validation via JWKS; issuer, audience, and expiry enforced on every request
- **Role-Based Access Control** — `user` and `admin` roles enforced at controller and domain policy levels
- **Anti-replay protection** — `X-Nonce` + `X-Request-Timestamp` with configurable window
- **Rate limiting** — per-IP (global) and per-user (post-auth), stricter limits on auth and chat endpoints
- **Secure HTTP headers** — CSP, HSTS, X-Frame-Options, X-Content-Type-Options, etc.
- **Request size limiting** — configurable max body bytes (default 1 MB)
- **Strict Content-Type enforcement** — only `application/json` accepted on mutating requests
- **Request timeout** — configurable; returns 504 if exceeded
- **XSS sanitization** — `bluemonday.StrictPolicy()` applied to all user input before persistence
- **No open endpoints** — every API route requires a valid JWT

### Observability
- **Structured JSON logging** with zerolog — request ID, user ID, latency on every request
- **Health endpoint** (`GET /health`) — DB connectivity check, version, uptime
- **Admin metrics endpoint** (`GET /api/v1/admin/metrics`) — usage statistics, admin-only



## Project Structure

```
secure-ai-chat-backend/
├── cmd/api/main.go                     # Entry point — composition root
├── docs/                               # Auto-generated Swagger spec (swag init)
├── internal/
│   ├── shared/
│   │   ├── config/        config.go    # Viper-based env config
│   │   ├── database/      database.go  # sqlx wrapper, migrations
│   │   ├── logger/        logger.go    # zerolog setup
│   │   ├── errors/        errors.go    # Typed AppError with HTTP codes
│   │   ├── response/      response.go  # JSON envelope helpers
│   │   ├── auth/
│   │   │   ├── claims.go              # JWT claims struct + context helpers
│   │   │   ├── middleware.go          # Auth0 RS256 JWKS validator
│   │   │   └── rbac.go               # RequireRole middleware
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
│   │   ├── repository/postgres.go     # Subscription + quota-side repos
│   │   ├── dto/request.go             # Input DTOs
│   │   └── controller/http.go         # chi HTTP handlers
│   └── admin/
│       └── controller/http.go         # Admin metrics + health endpoints
├── migrations/
│   ├── 000001_create_users.*
│   ├── 000002_create_subscriptions.*
│   ├── 000003_create_quota_usages.*
│   └── 000004_create_chat_messages.*
├── tests/
│   ├── unit/                          # Domain logic, quota math, subscription lifecycle
│   └── integration/                   # Middleware, auth (mocked provider), rate limiting
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
git clone https://github.com/Ammar022/secure-ai-chat-backend
cd secure-ai-chat-backend
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

The API accepts any valid Auth0 RS256 JWT. Two ways to get one:

---

#### Option A — Email / password (Resource Owner Password Grant)

Requires the **Password** grant type enabled on your Auth0 Application and a **Default Directory** (Username-Password-Authentication) set on your tenant.

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

---

#### Option B — Google OAuth (Authorization Code flow)

1. **Enable Google social connection** in Auth0 Dashboard → Authentication → Social → Google.
2. Add `http://localhost:3000/callback` (or any placeholder URL) to **Allowed Callback URLs** on your Application.
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

4. After login the browser redirects to `localhost:3000/callback?code=<AUTH_CODE>`. Copy the code (the page won't load — that's fine).
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

---

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

### 4. Explore the API

```
Swagger UI: http://localhost:8080/swagger/index.html
```

### 5. Regenerate API docs (after changing annotations)

```bash
# Regenerate docs/ package and swagger.json/swagger.yaml
swag init -g cmd/api/main.go -o docs --parseInternal --parseDependency
```

### 6. Run tests

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
| `AUTH0_AUDIENCE` | ✓ | Auth0 API identifier (e.g. `https://api`) |
| `AUTH0_ROLES_CLAIM` | — | Custom namespace for roles claim (default: `https://api.yourdomain.com/roles`) |
| `CORS_ALLOWED_ORIGINS` | — | Comma-separated allowed origins (default: `http://localhost:3000`) |
| `RATE_LIMIT_IP_RPM` | — | IP rate limit req/min (default: 100) |
| `RATE_LIMIT_USER_RPM` | — | Per-user rate limit req/min (default: 50) |
| `AI_LATENCY_MIN_MS` | — | Mock AI min latency ms (default: 500) |
| `AI_LATENCY_MAX_MS` | — | Mock AI max latency ms (default: 2000) |
| `RENEWAL_INTERVAL_MINUTES` | — | Subscription renewal job interval (default: 60) |

---

## Security Model

### Authentication flow

```
Client obtains JWT from Auth0 (dashboard test tab, M2M, or SPA login)

API request → Authorization: Bearer <token>
  → Extract token from header
  → Fetch Auth0 JWKS (cached, auto-refreshed every 15 min)
  → Validate RS256 signature
  → Verify issuer (AUTH0_DOMAIN) + audience (AUTH0_AUDIENCE) + expiry
  → Extract subject, email, roles from claims
  → UserSync middleware — upsert local user record, populate InternalUserID
  → Claims injected into request context
```

### Authorization layers

1. **JWT middleware** — every `/api/v1` request must carry a valid token
2. **RBAC middleware** — `RequireRole(admin)` guards admin routes
3. **Domain policy** — pure functions enforce ownership (user can only touch own resources)

### Anti-replay

Every mutating request must include:
- `X-Request-Timestamp` — Unix epoch; rejected if outside ±`SECURITY_ANTI_REPLAY_WINDOW_SEC` seconds
- `X-Nonce` — unique string per request; cached server-side; duplicates rejected with 401

### Rate limiting

- **Global IP limit** — applied before JWT validation (default: 120 req/min)
- **Per-user limit** — applied after user identification (default: 60 req/min)
- **Chat-specific limit** — tighter budget on the AI endpoint (default: 20 req/min)

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
