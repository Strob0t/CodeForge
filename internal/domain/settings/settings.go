// Package settings defines the domain types for application settings.
package settings

import (
	"encoding/json"
	"time"
)

// Setting represents a key-value configuration setting.
type Setting struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// UpdateRequest holds the fields to update one or more settings.
type UpdateRequest struct {
	Settings map[string]json.RawMessage `json:"settings"`
}
