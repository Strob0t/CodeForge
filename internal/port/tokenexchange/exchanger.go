// Package tokenexchange defines the port for token exchange operations.
package tokenexchange

import (
	"context"
	"time"
)

// Exchanger abstracts token exchange operations (e.g., GitHub Copilot).
type Exchanger interface {
	ExchangeToken(ctx context.Context) (token string, expiry time.Time, err error)
}
