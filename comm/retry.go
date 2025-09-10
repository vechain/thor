// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"context"
	"math/rand"
	"time"
)

// retryWithBackoff retries fn up to maxAttempts with exponential backoff.
// The delay starts at initialDelay and doubles each time up to maxDelay.
// A small jitter (±10%) is added to spread retries.
// The function is cancellable via ctx.
func retryWithBackoff(ctx context.Context, maxRetry int, initialDelay, maxDelay time.Duration, fn func(context.Context) error) error {
	if maxRetry <= 0 {
		return fn(ctx)
	}

	// Create a per-call RNG for jitter
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) // #nosec G404

	delay := initialDelay
	var lastErr error
	for attempt := 1; attempt <= maxRetry; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := fn(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if attempt == maxRetry {
			break
		}

		// jitter in [0.9, 1.1]
		jitter := 0.9 + 0.2*rng.Float64()
		sleepFor := min(time.Duration(float64(delay)*jitter), maxDelay)

		timer := time.NewTimer(sleepFor)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}

		// Exponential backoff with cap
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
	return lastErr
}
