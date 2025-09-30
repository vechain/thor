// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryWithBackoff_SucceedsImmediate(t *testing.T) {
	t.Parallel()
	var attempts int
	err := retryWithBackoff(context.Background(), 5, 10*time.Millisecond, 50*time.Millisecond, func(ctx context.Context) error {
		attempts++
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, attempts, "should not retry when first attempt succeeds")
}

func TestRetryWithBackoff_SucceedsAfterRetries(t *testing.T) {
	t.Parallel()
	var attempts int
	failFirst := 2
	err := retryWithBackoff(context.Background(), 5, 10*time.Millisecond, 50*time.Millisecond, func(ctx context.Context) error {
		attempts++
		if attempts <= failFirst {
			return errors.New("transient error")
		}
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, failFirst+1, attempts, "should retry until success")
}

func TestRetryWithBackoff_ExhaustsAttempts(t *testing.T) {
	t.Parallel()
	var attempts int
	maxAttempts := 3
	start := time.Now()
	err := retryWithBackoff(context.Background(), maxAttempts, 10*time.Millisecond, 40*time.Millisecond, func(ctx context.Context) error {
		attempts++
		return errors.New("always fails")
	})
	elapsed := time.Since(start)
	assert.Error(t, err)
	assert.Equal(t, maxAttempts, attempts)
	// Expect at least (maxAttempts-1) sleeps with jitter; allow generous lower bound
	assert.GreaterOrEqual(t, elapsed, 10*time.Millisecond)
}

func TestRetryWithBackoff_ContextCancelledBeforeStart(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var attempts int
	err := retryWithBackoff(ctx, 5, 10*time.Millisecond, 50*time.Millisecond, func(ctx context.Context) error {
		attempts++
		return nil
	})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
	assert.Equal(t, 0, attempts, "fn should not be called when context already cancelled")
}

func TestRetryWithBackoff_ContextCancelledDuringSleep(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var attempts int
	done := make(chan struct{})
	go func() {
		// Cancel during the first backoff
		time.Sleep(30 * time.Millisecond)
		cancel()
		close(done)
	}()

	start := time.Now()
	err := retryWithBackoff(ctx, 5, 200*time.Millisecond, 200*time.Millisecond, func(ctx context.Context) error {
		attempts++
		return errors.New("transient")
	})
	<-done
	elapsed := time.Since(start)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
	assert.Equal(t, 1, attempts, "should cancel during first sleep, so only one attempt")
	assert.Less(t, elapsed, 200*time.Millisecond, "should return before completing first sleep")
}

func TestRetryWithBackoff_InitialDelayZero(t *testing.T) {
	t.Parallel()
	var attempts int
	maxAttempts := 3
	start := time.Now()
	err := retryWithBackoff(context.Background(), maxAttempts, 0, 0, func(ctx context.Context) error {
		attempts++
		return errors.New("boom")
	})
	elapsed := time.Since(start)
	assert.Error(t, err)
	assert.Equal(t, maxAttempts, attempts)
	assert.Less(t, elapsed, 50*time.Millisecond)
}

func TestRetryWithBackoff_MaxDelayCap(t *testing.T) {
	t.Parallel()
	var attempts int
	successAt := 4 // will perform 3 sleeps before succeeding
	start := time.Now()
	err := retryWithBackoff(context.Background(), successAt, 20*time.Millisecond, 30*time.Millisecond, func(ctx context.Context) error {
		attempts++
		if attempts < successAt {
			return errors.New("retry")
		}
		return nil
	})
	elapsed := time.Since(start)
	assert.NoError(t, err)
	assert.Equal(t, successAt, attempts)
	// Expect at least ~20 + 30 + 30 ms minus jitter; allow generous lower bound
	assert.GreaterOrEqual(t, elapsed, 50*time.Millisecond)
}
