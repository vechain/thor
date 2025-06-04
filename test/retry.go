package test

import (
	"fmt"
	"time"
)

func Retry(fn func() error, retryPeriod, maxWaitTime time.Duration) error {
	startTime := time.Now()
	for {
		err := fn()
		if err == nil {
			// If the function succeeds, return nil error
			return nil
		}

		if time.Since(startTime) > maxWaitTime {
			// If maxWaitTime has been exceeded, return the last error
			return fmt.Errorf("retry timeout, latest err: %w", err)
		}

		// Wait for the retryPeriod before retrying
		time.Sleep(retryPeriod)
	}
}
