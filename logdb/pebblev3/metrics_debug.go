//go:build logdb_debug

// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebblev3

import (
	"sync/atomic"
)

// IteratorMetrics tracks iterator operation counts for debugging
type IteratorMetrics struct {
	NextCalls      int64
	SeekGECalls    int64
	SeekLTCalls    int64
	HeapOperations int64
}

var globalMetrics = &IteratorMetrics{}

// recordNext increments the Next call counter
func recordNext() {
	atomic.AddInt64(&globalMetrics.NextCalls, 1)
}

// recordSeekGE increments the SeekGE call counter
func recordSeekGE() {
	atomic.AddInt64(&globalMetrics.SeekGECalls, 1)
}

// recordSeekLT increments the SeekLT call counter
func recordSeekLT() {
	atomic.AddInt64(&globalMetrics.SeekLTCalls, 1)
}

// recordHeapOp increments the heap operation counter
func recordHeapOp() {
	atomic.AddInt64(&globalMetrics.HeapOperations, 1)
}

// getMetrics returns the current metrics snapshot
func getMetrics() *IteratorMetrics {
	return &IteratorMetrics{
		NextCalls:      atomic.LoadInt64(&globalMetrics.NextCalls),
		SeekGECalls:    atomic.LoadInt64(&globalMetrics.SeekGECalls),
		SeekLTCalls:    atomic.LoadInt64(&globalMetrics.SeekLTCalls),
		HeapOperations: atomic.LoadInt64(&globalMetrics.HeapOperations),
	}
}

// resetMetrics resets all counters to zero
func resetMetrics() {
	atomic.StoreInt64(&globalMetrics.NextCalls, 0)
	atomic.StoreInt64(&globalMetrics.SeekGECalls, 0)
	atomic.StoreInt64(&globalMetrics.SeekLTCalls, 0)
	atomic.StoreInt64(&globalMetrics.HeapOperations, 0)
}