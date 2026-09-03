// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

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
