package domain

import (
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

type User struct {
	ID         uuid.UUID `db:"id"`
	ExternalID string    `db:"external_id"`
	Email      string    `db:"email"`
	Role       Role      `db:"role"`

	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

func NewUser(externalID, email string) *User {
	return &User{
		ID:         uuid.New(),
		ExternalID: externalID,
		Email:      email,
		Role:       RoleUser,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
}
