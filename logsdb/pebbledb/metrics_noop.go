//go:build !logsdb_debug

// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebbledb

// IteratorMetrics is a no-op type for production builds
type IteratorMetrics struct{}

// No-op implementations for production builds
func recordNext()   {}
func recordSeekGE() {}
func recordSeekLT() {}
func recordHeapOp() {}
func resetMetrics() {}

// getMetrics returns nil in production builds
func getMetrics() *IteratorMetrics {
	return nil
}
