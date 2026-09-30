package service

import (
	"context"
	"sync"
	"time"
)

// startPeriodic runs fn every interval - and once right away when runAtStart -
// until ctx ends or the returned stop is called. stop cancels a call under way
// and waits for it to end, so a caller can stop before closing what fn uses.
func startPeriodic(ctx context.Context, interval time.Duration, runAtStart bool, fn func(ctx context.Context)) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Go(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		if runAtStart {
			fn(ctx)
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fn(ctx)
			}
		}
	})
	return func() {
		cancel()
		wg.Wait()
	}
}
