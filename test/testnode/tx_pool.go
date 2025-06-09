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

// Pool mines a block when requested.
type Pool struct {
	txs       tx.Transactions
	validator genesis.DevAccount
	chain     *testchain.Chain

	mutex  sync.Mutex
	scope  event.SubscriptionScope
	txFeed event.Feed
}

func (m *Pool) Get(txID thor.Bytes32) *tx.Transaction {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, tx := range m.txs {
		if tx.ID() == txID {
			return tx
		}
	}
	return nil
}

// AddLocal adds a transaction to the pool without minting
func (m *Pool) AddLocal(trx *tx.Transaction) error {
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
	return nil
}

// MintTransactions mints all pending transactions in the pool
func (m *Pool) MintTransactions() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if len(m.txs) == 0 {
		return nil
	}

	// Mint all transactions in the pool
	err := m.chain.MintBlock(m.validator, m.txs...)
	if err != nil {
		return err
	}

	// Clear the pool after successful minting
	m.txs = make(tx.Transactions, 0)
	return nil
}

func (m *Pool) Dump() tx.Transactions {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return m.txs
}

func (m *Pool) Len() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return len(m.txs)
}

func (m *Pool) SubscribeTxEvent(ch chan *txpool.TxEvent) event.Subscription {
	return m.scope.Track(m.txFeed.Subscribe(ch))
}
