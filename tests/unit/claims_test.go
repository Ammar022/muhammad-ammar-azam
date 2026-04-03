package unit

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/auth"
)

// ── HasRole ───────────────────────────────────────────────────────────────────

func TestClaims_HasRole_EmptyRoles_ReturnsFalse(t *testing.T) {
	c := &auth.Claims{Roles: []auth.Role{}}
	assert.False(t, c.HasRole(auth.RoleUser))
	assert.False(t, c.HasRole(auth.RoleAdmin))
}

func TestClaims_HasRole_SingleMatchingRole_ReturnsTrue(t *testing.T) {
	c := &auth.Claims{Roles: []auth.Role{auth.RoleUser}}
	assert.True(t, c.HasRole(auth.RoleUser))
}

func TestClaims_HasRole_SingleNonMatchingRole_ReturnsFalse(t *testing.T) {
	c := &auth.Claims{Roles: []auth.Role{auth.RoleUser}}
	assert.False(t, c.HasRole(auth.RoleAdmin))
}

func TestClaims_HasRole_MultipleRoles_MatchesAny(t *testing.T) {
	c := &auth.Claims{Roles: []auth.Role{auth.RoleUser, auth.RoleAdmin}}
	assert.True(t, c.HasRole(auth.RoleUser))
	assert.True(t, c.HasRole(auth.RoleAdmin))
}

func TestClaims_HasRole_UnknownRole_ReturnsFalse(t *testing.T) {
	c := &auth.Claims{Roles: []auth.Role{auth.RoleUser}}
	assert.False(t, c.HasRole(auth.Role("superuser")))
}

// ── IsAdmin ───────────────────────────────────────────────────────────────────

func TestClaims_IsAdmin_WithAdminRole_ReturnsTrue(t *testing.T) {
	c := &auth.Claims{Roles: []auth.Role{auth.RoleAdmin}}
	assert.True(t, c.IsAdmin())
}

func TestClaims_IsAdmin_WithOnlyUserRole_ReturnsFalse(t *testing.T) {
	c := &auth.Claims{Roles: []auth.Role{auth.RoleUser}}
	assert.False(t, c.IsAdmin())
}

func TestClaims_IsAdmin_WithNoRoles_ReturnsFalse(t *testing.T) {
	c := &auth.Claims{Roles: []auth.Role{}}
	assert.False(t, c.IsAdmin())
}

func TestClaims_IsAdmin_WithBothRoles_ReturnsTrue(t *testing.T) {
	c := &auth.Claims{Roles: []auth.Role{auth.RoleUser, auth.RoleAdmin}}
	assert.True(t, c.IsAdmin())
}

// ── ClaimsFromContext ─────────────────────────────────────────────────────────

func TestClaimsFromContext_WithNoClaims_ReturnsNilFalse(t *testing.T) {
	ctx := context.Background()
	claims, ok := auth.ClaimsFromContext(ctx)
	assert.False(t, ok)
	assert.Nil(t, claims)
}

func TestClaimsFromContext_WithClaims_ReturnsClaims(t *testing.T) {
	expected := &auth.Claims{
		Subject: "auth0|test123",
		Email:   "test@example.com",
		Roles:   []auth.Role{auth.RoleUser},
	}
	ctx := auth.WithClaims(context.Background(), expected)

	claims, ok := auth.ClaimsFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, expected.Subject, claims.Subject)
	assert.Equal(t, expected.Email, claims.Email)
}

func TestClaimsFromContext_WithAdminClaims_CanCheckRole(t *testing.T) {
	adminClaims := &auth.Claims{
		Subject:        "admin-user",
		Roles:          []auth.Role{auth.RoleAdmin},
		InternalUserID: uuid.New(),
	}
	ctx := auth.WithClaims(context.Background(), adminClaims)

	claims, ok := auth.ClaimsFromContext(ctx)
	require.True(t, ok)
	assert.True(t, claims.IsAdmin())
}

// ── WithClaims / round-trip ───────────────────────────────────────────────────

func TestWithClaims_RoundTrip_PreservesAllFields(t *testing.T) {
	id := uuid.New()
	original := &auth.Claims{
		Subject:        "auth0|abc",
		Email:          "user@example.com",
		Roles:          []auth.Role{auth.RoleUser, auth.RoleAdmin},
		InternalUserID: id,
	}

	ctx := auth.WithClaims(context.Background(), original)
	retrieved, ok := auth.ClaimsFromContext(ctx)
	require.True(t, ok)

	assert.Equal(t, original.Subject, retrieved.Subject)
	assert.Equal(t, original.Email, retrieved.Email)
	assert.Equal(t, original.InternalUserID, retrieved.InternalUserID)
	assert.Equal(t, original.Roles, retrieved.Roles)
}

func TestWithClaims_OverwritesPreviousValue(t *testing.T) {
	first := &auth.Claims{Subject: "first-user"}
	second := &auth.Claims{Subject: "second-user"}

	ctx := auth.WithClaims(context.Background(), first)
	ctx = auth.WithClaims(ctx, second)

	claims, ok := auth.ClaimsFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "second-user", claims.Subject)
}

// ── MustClaimsFromContext ─────────────────────────────────────────────────────

func TestMustClaimsFromContext_WithClaims_ReturnsClaims(t *testing.T) {
	c := &auth.Claims{Subject: "test"}
	ctx := auth.WithClaims(context.Background(), c)

	assert.NotPanics(t, func() {
		result := auth.MustClaimsFromContext(ctx)
		assert.Equal(t, "test", result.Subject)
	})
}

func TestMustClaimsFromContext_WithNoClaims_Panics(t *testing.T) {
	ctx := context.Background()
	assert.Panics(t, func() {
		_ = auth.MustClaimsFromContext(ctx)
	})
}
