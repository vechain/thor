// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import "sync"

// EthNonce manages the sequential 0x02 nonce locally. Spec 3 requires each
// 0x02 tx to use the current account nonce; this struct encapsulates the
// counter and serializes access across goroutines (txblast is single-threaded
// per batch, but the mutex is defensive).
type EthNonce struct {
	mu      sync.Mutex
	current uint64
}

func NewEthNonce(initial uint64) *EthNonce { return &EthNonce{current: initial} }

// Take returns the current nonce and increments internally.
func (n *EthNonce) Take() uint64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	v := n.current
	n.current++
	return v
}

// Reset overwrites the counter (used when recovering from nonce desync).
func (n *EthNonce) Reset(v uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.current = v
}
