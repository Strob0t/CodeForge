// Package vcsaccount defines the domain types for VCS account management.
package vcsaccount

import "time"

// VCSAccount represents a stored VCS provider credential.
type VCSAccount struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	Provider       string    `json:"provider"`    // github, gitlab, gitea, bitbucket
	Label          string    `json:"label"`       // user-friendly name
	ServerURL      string    `json:"server_url"`  // for self-hosted (empty = cloud default)
	AuthMethod     string    `json:"auth_method"` // token, ssh, oauth
	EncryptedToken []byte    `json:"-"`           // never expose in JSON
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CreateRequest holds the fields to create a new VCS account.
type CreateRequest struct {
	Provider   string `json:"provider"`
	Label      string `json:"label"`
	ServerURL  string `json:"server_url"`
	AuthMethod string `json:"auth_method"`
	Token      string `json:"token"` // plaintext, encrypted before storage
}
