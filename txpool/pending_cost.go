// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"math/big"
	"sync"

	"github.com/vechain/thor/v2/thor"
)

// PendingCostTracker is a shared per-payer soft reservation of prepaid VTHO
// (gas × effectiveGasPrice) across VeChainPool and EthPool.
//
// Sub-pools must not call each other; both reserve/release against this ledger
// so a payer cannot be over-committed when submitting cross-family txs.
type PendingCostTracker struct {
	mu   sync.RWMutex
	cost map[thor.Address]*big.Int
}

// NewPendingCostTracker creates an empty shared pending-cost ledger.
func NewPendingCostTracker() *PendingCostTracker {
	return &PendingCostTracker{
		cost: make(map[thor.Address]*big.Int),
	}
}

// Pending returns a copy of the reserved prepaid VTHO for payer.
func (t *PendingCostTracker) Pending(payer thor.Address) *big.Int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if cost := t.cost[payer]; cost != nil {
		return new(big.Int).Set(cost)
	}
	return new(big.Int)
}

// Reserve atomically validates needs = Pending(payer)+amount via canPay, then
// records the reservation. amount must be non-nil and positive.
func (t *PendingCostTracker) Reserve(payer thor.Address, amount *big.Int, canPay func(needs *big.Int) error) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	needs := new(big.Int).Set(amount)
	if pending := t.cost[payer]; pending != nil {
		needs = new(big.Int).Add(pending, amount)
	}
	if err := canPay(needs); err != nil {
		return err
	}
	t.cost[payer] = needs
	return nil
}

// Release subtracts amount from payer's reservation. No-op if nothing reserved.
func (t *PendingCostTracker) Release(payer thor.Address, amount *big.Int) {
	if amount == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	pending := t.cost[payer]
	if pending == nil {
		return
	}
	if pending.Cmp(amount) <= 0 {
		delete(t.cost, payer)
		return
	}
	t.cost[payer] = new(big.Int).Sub(pending, amount)
}
