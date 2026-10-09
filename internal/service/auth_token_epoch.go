package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/user"
)

// Validating an access token compares its epoch with the user's current one
// (KI-143). The current epoch is cached briefly: a change made on another
// replica takes effect there within tokenEpochCacheTTL, one made here at once.
const (
	tokenEpochCacheTTL = 5 * time.Second
	tokenEpochCacheMax = 10_000
)

var (
	errTokenWithoutEpoch = errors.New("token predates token epochs, sign in again")
	errTokenEpochStale   = errors.New("token has been invalidated, sign in again")
	errTokenUserGone     = errors.New("token user no longer exists")
)

// epochEntry is a cached token epoch; gone records a user that does not
// exist (deleted or erased), so a token of such a user is not looked up on
// every request.
type epochEntry struct {
	tenantID string
	epoch    int64
	gone     bool
	expires  time.Time
}

// tokenEpochCache holds users' token epochs for a short time, at most max
// entries: when it is full, expired entries go first, then arbitrary ones.
// gen counts forgets: a value read from the store is cached only if no forget
// happened since the read began, so a lookup racing a raise cannot put the
// old epoch back after the raise dropped it.
type tokenEpochCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	max     int
	now     func() time.Time
	gen     uint64
	entries map[string]epochEntry // by user ID
}

func newTokenEpochCache(ttl time.Duration, maxEntries int) *tokenEpochCache {
	return &tokenEpochCache{ttl: ttl, max: maxEntries, now: time.Now, entries: make(map[string]epochEntry)}
}

func (c *tokenEpochCache) get(userID, tenantID string) (epochEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[userID]
	if !ok || e.tenantID != tenantID || !c.now().Before(e.expires) {
		return epochEntry{}, false
	}
	return e, true
}

// generation is taken before a store read and passed to putIfCurrent.
func (c *tokenEpochCache) generation() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

// putIfCurrent caches a value read from the store unless a forget happened
// since generation gen was taken.
func (c *tokenEpochCache) putIfCurrent(gen uint64, userID, tenantID string, epoch int64, gone bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.gen {
		return
	}
	if _, ok := c.entries[userID]; !ok && len(c.entries) >= c.max {
		c.evictLocked()
	}
	c.entries[userID] = epochEntry{tenantID: tenantID, epoch: epoch, gone: gone, expires: c.now().Add(c.ttl)}
}

func (c *tokenEpochCache) evictLocked() {
	now := c.now()
	for id, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, id)
		}
	}
	for id := range c.entries {
		if len(c.entries) < c.max {
			return
		}
		delete(c.entries, id)
	}
}

func (c *tokenEpochCache) forget(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	delete(c.entries, userID)
}

func (c *tokenEpochCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// connectionDropper closes a user's WebSocket connections (ws.Hub).
type connectionDropper interface {
	DropUser(userID string) int
}

// EndUserSessions applies the end of a user's tokens on this replica at once:
// it drops the cached token epoch, so a raised epoch or a deleted user takes
// effect, and closes the user's WebSocket connections.
func (t *TokenManager) EndUserSessions(userID string) {
	t.epochs.forget(userID)
	if t.connections != nil {
		t.connections.DropUser(userID)
	}
}

// checkTokenEpoch refuses a token without an epoch, one whose user does not
// exist and one whose epoch is not the user's current one. A failed lookup
// refuses the token too (fail closed) and is not cached.
func (t *TokenManager) checkTokenEpoch(ctx context.Context, claims *user.TokenClaims) error {
	if claims.TokenEpoch == nil {
		return errTokenWithoutEpoch
	}
	e, ok := t.epochs.get(claims.UserID, claims.TenantID)
	if !ok {
		gen := t.epochs.generation()
		epoch, err := t.store.GetUserTokenEpoch(ctx, claims.UserID, claims.TenantID)
		switch {
		case errors.Is(err, domain.ErrNotFound):
			e = epochEntry{gone: true}
		case err != nil:
			slog.Error("token epoch check failed, denying token", "user_id", claims.UserID, "error", err)
			return errors.New("unable to verify token status")
		default:
			e = epochEntry{epoch: epoch}
		}
		t.epochs.putIfCurrent(gen, claims.UserID, claims.TenantID, e.epoch, e.gone)
	}
	if e.gone {
		return errTokenUserGone
	}
	if e.epoch != *claims.TokenEpoch {
		return errTokenEpochStale
	}
	return nil
}
