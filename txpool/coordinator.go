// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"github.com/ethereum/go-ethereum/event"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// TxPoolCoordinator is the façade over VeChainPool and EthPool.
// Callers should depend on the Pool interface backed by this type.
type TxPoolCoordinator struct {
	vechain *VeChainPool
	eth     *EthPool
	costs   *PendingCostTracker
}

var _ Pool = (*TxPoolCoordinator)(nil)

// NewCoordinator creates both sub-pools sharing one PendingCostTracker.
// Close must be called at shutdown.
func NewCoordinator(repo *chain.Repository, stater *state.Stater, options Options, forkConfig *thor.ForkConfig) *TxPoolCoordinator {
	costs := NewPendingCostTracker()
	return &TxPoolCoordinator{
		costs:   costs,
		vechain: newVeChainPool(repo, stater, options, forkConfig, costs),
		eth:     newEthPool(repo, stater, options, forkConfig, costs),
	}
}

// route selects the sub-pool for a transaction.
//
// TODO(eth-txpool): flip to route by newTx.IsEthereumTx() once EthPool admission
// is implemented. Until then, all families go to VeChainPool to preserve current
// behavior.
func (c *TxPoolCoordinator) route(newTx *tx.Transaction) Pool {
	_ = newTx
	return c.vechain
}

func (c *TxPoolCoordinator) Get(txID thor.Bytes32) *tx.Transaction {
	if trx := c.vechain.Get(txID); trx != nil {
		return trx
	}
	return c.eth.Get(txID)
}

func (c *TxPoolCoordinator) GetByHash(hash thor.Bytes32) *tx.Transaction {
	if trx := c.vechain.GetByHash(hash); trx != nil {
		return trx
	}
	return c.eth.GetByHash(hash)
}

func (c *TxPoolCoordinator) AddRemote(newTx *tx.Transaction) error {
	return c.route(newTx).AddRemote(newTx)
}

func (c *TxPoolCoordinator) ReinjectFromFork(newTx *tx.Transaction) error {
	return c.route(newTx).ReinjectFromFork(newTx)
}

func (c *TxPoolCoordinator) AddLocal(newTx *tx.Transaction) error {
	return c.route(newTx).AddLocal(newTx)
}

func (c *TxPoolCoordinator) StrictlyAdd(newTx *tx.Transaction) error {
	return c.route(newTx).StrictlyAdd(newTx)
}

func (c *TxPoolCoordinator) Remove(txHash thor.Bytes32, txID thor.Bytes32) bool {
	if c.vechain.Remove(txHash, txID) {
		return true
	}
	return c.eth.Remove(txHash, txID)
}

func (c *TxPoolCoordinator) Dump() tx.Transactions {
	vechainTxs := c.vechain.Dump()
	ethTxs := c.eth.Dump()
	if len(ethTxs) == 0 {
		return vechainTxs
	}
	out := make(tx.Transactions, 0, len(vechainTxs)+len(ethTxs))
	out = append(out, vechainTxs...)
	out = append(out, ethTxs...)
	return out
}

func (c *TxPoolCoordinator) Len() int {
	return c.vechain.Len() + c.eth.Len()
}

func (c *TxPoolCoordinator) SubscribeTxEvent(ch chan *TxEvent) event.Subscription {
	// Scaffold: forward VeChain events only. Dual-pool event relay lands with
	// the EthPool admission flip.
	return c.vechain.SubscribeTxEvent(ch)
}

func (c *TxPoolCoordinator) Executables() tx.Transactions {
	// Scaffold: no k-way merge yet; EthPool returns empty.
	return c.vechain.Executables()
}

func (c *TxPoolCoordinator) Fill(txs tx.Transactions) {
	// Temporary: all families still live in VeChainPool.
	c.vechain.Fill(txs)
}

func (c *TxPoolCoordinator) PoolNonce(addr thor.Address) uint64 {
	return c.eth.PoolNonce(addr)
}

func (c *TxPoolCoordinator) Close() {
	c.vechain.Close()
	c.eth.Close()
}
