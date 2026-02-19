// Package user defines the user domain model for authentication and authorization.
package user

import (
	"errors"
	"net/mail"
	"time"
)

// Role represents the authorization level of a user.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

// ValidRoles is the set of all valid user roles.
var ValidRoles = map[Role]bool{
	RoleAdmin:  true,
	RoleEditor: true,
	RoleViewer: true,
}

// User represents a registered user within a tenant.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"` // never serialized
	Role         Role      `json:"role"`
	TenantID     string    `json:"tenant_id"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateRequest is the input for registering a new user.
type CreateRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"` //nolint:gosec // request field, not a hardcoded secret
	Role     Role   `json:"role"`
	TenantID string `json:"tenant_id"`
}

// Validate checks that the CreateRequest has all required fields.
func (r *CreateRequest) Validate() error {
	if r.Email == "" {
		return errors.New("email is required")
	}
	if _, err := mail.ParseAddress(r.Email); err != nil {
		return errors.New("invalid email format")
	}
	if r.Name == "" {
		return errors.New("name is required")
	}
	if r.Password == "" {
		return errors.New("password is required")
	}
	if len(r.Password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if !ValidRoles[r.Role] {
		return errors.New("invalid role: must be admin, editor, or viewer")
	}
	return nil
}

// UpdateRequest is the input for updating an existing user.
type UpdateRequest struct {
	Name    string `json:"name,omitempty"`
	Role    Role   `json:"role,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// LoginRequest is the input for user authentication.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"` //nolint:gosec // request field, not a hardcoded secret
}

// Validate checks that the LoginRequest has all required fields.
func (r *LoginRequest) Validate() error {
	if r.Email == "" {
		return errors.New("email is required")
	}
	if r.Password == "" {
		return errors.New("password is required")
	}
	return nil
}

// LoginResponse is returned after successful authentication.
type LoginResponse struct {
	AccessToken string `json:"access_token"` //nolint:gosec // response field, not a hardcoded secret
	ExpiresIn   int    `json:"expires_in"`   // seconds until access token expires
	User        User   `json:"user"`
}

// TokenClaims contains the JWT payload fields.
type TokenClaims struct {
	UserID   string `json:"sub"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     Role   `json:"role"`
	TenantID string `json:"tid"`
	IssuedAt int64  `json:"iat"`
	Expiry   int64  `json:"exp"`
}

// RefreshToken represents a stored refresh token.
type RefreshToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}
