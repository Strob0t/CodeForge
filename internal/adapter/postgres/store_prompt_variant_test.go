package postgres_test

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/service"
)

// Compile-time interface compliance checks.
// These fail at build time if *postgres.Store does not implement the interface.
var (
	_ service.PromptEvolutionStore = (*postgres.Store)(nil)
	_ service.PromptVariantStore   = (*postgres.Store)(nil)
	_ service.PromptScoreStore     = (*postgres.Store)(nil)
)

func TestPromptVariantStore_InterfaceCompliance(t *testing.T) {
	t.Parallel()
	// Compile-time var _ declarations above are the actual checks.
	// This test simply ensures the file is included in test runs.
}

func TestPromptScoreStore_InterfaceCompliance(t *testing.T) {
	t.Parallel()
	// Compile-time var _ declaration above is the actual check.
}
