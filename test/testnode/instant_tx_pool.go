// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package testnode

import (
	"sync"

	"github.com/ethereum/go-ethereum/event"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

// instantMintPool instantly mines a block when adding a transaction to the pool.
type instantMintPool struct {
	txs       tx.Transactions
	validator genesis.DevAccount
	chain     *testchain.Chain

	mutex  sync.Mutex
	scope  event.SubscriptionScope
	txFeed event.Feed
}

var _ txpool.Pool = (*instantMintPool)(nil)

func (m *instantMintPool) Get(txID thor.Bytes32) *tx.Transaction {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, tx := range m.txs {
		if tx.ID() == txID {
			return tx
		}
	}
	return nil
}

func (m *instantMintPool) Add(newTx *tx.Transaction) error {
	return m.AddLocal(newTx)
}

func (m *instantMintPool) AddLocal(trx *tx.Transaction) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.txs == nil {
		m.txs = make(tx.Transactions, 0)
	}
	m.txs = append(m.txs, trx)
	executable := true
	m.txFeed.Send(&txpool.TxEvent{
		Tx:         trx,
		Executable: &executable,
	})
	return m.chain.MintBlock(m.validator, trx)
}

func (m *instantMintPool) StrictlyAdd(newTx *tx.Transaction) error {
	return m.Add(newTx)
}

func (m *instantMintPool) Dump() tx.Transactions {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return m.txs
}

func (m *instantMintPool) Len() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return len(m.txs)
}

func (m *instantMintPool) SubscribeTxEvent(ch chan *txpool.TxEvent) event.Subscription {
	return m.scope.Track(m.txFeed.Subscribe(ch))
}

func (m *instantMintPool) Remove(txHash thor.Bytes32, txID thor.Bytes32) bool { return true }

func (m *instantMintPool) Close() {}

func (m *instantMintPool) Fill(txs tx.Transactions) {}

func (m *instantMintPool) Executables() tx.Transactions {
	return m.txs
}
