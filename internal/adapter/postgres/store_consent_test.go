package postgres_test

import (
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// KI-149: an unknown consent purpose is a resource that does not exist, so
// PUT /me/consent/{purposeID} answers 404, not 500.
func TestStore_GetConsentPurpose_UnknownIsNotFound(t *testing.T) {
	store := setupStore(t)
	ctx := ctxWithTenant(t, createTestTenant(t, store))

	if _, err := store.GetConsentPurpose(ctx, "no-such-purpose"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetConsentPurpose(unknown) = %v, want ErrNotFound", err)
	}
}
