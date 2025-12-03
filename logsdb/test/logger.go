// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package test

import (
	"fmt"
	"time"
)

// logf logs a formatted message with timestamp if verbose mode is enabled
func logf(format string, args ...interface{}) {
	if *Verbose {
		timestamp := time.Now().Format("15:04:05.000")
		fmt.Printf("[%s] "+format+"\n", append([]interface{}{timestamp}, args...)...)
	}
}

// logln logs a message with timestamp if verbose mode is enabled
func logln(msg string) {
	if *Verbose {
		timestamp := time.Now().Format("15:04:05.000")
		fmt.Printf("[%s] %s\n", timestamp, msg)
	}
}

// alwaysLogf always logs (used for important messages)
func alwaysLogf(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05.000")
	fmt.Printf("[%s] "+format+"\n", append([]interface{}{timestamp}, args...)...)
}