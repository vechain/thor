// Copyright (c) 2026 The VeChainThor developers

package txpool

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func newMergeTestEntry(priority, timeAdded int64) executableTx {
	return executableTx{
		tx:               &tx.Transaction{},
		priorityGasPrice: big.NewInt(priority),
		timeAdded:        timeAdded,
	}
}

func transactionsOf(entries ...executableTx) tx.Transactions {
	txs := make(tx.Transactions, 0, len(entries))
	for _, entry := range entries {
		txs = append(txs, entry.tx)
	}
	return txs
}

func TestMergeExecutables(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		assert.Nil(t, mergeExecutables(nil, ethExecutablesSnapshot{}))
	})

	t.Run("one source", func(t *testing.T) {
		first := newMergeTestEntry(20, 1)
		second := newMergeTestEntry(10, 1)
		assert.Equal(t, transactionsOf(first, second), mergeExecutables(
			[]executableTx{first, second},
			ethExecutablesSnapshot{},
		))
	})

	t.Run("priority merge preserves each Ethereum source order", func(t *testing.T) {
		vechainHigh := newMergeTestEntry(100, 1)
		vechainLow := newMergeTestEntry(30, 1)
		senderANonce0 := newMergeTestEntry(10, 1)
		senderANonce1 := newMergeTestEntry(1_000, 1)
		senderBNonce0 := newMergeTestEntry(50, 1)
		senderBNonce1 := newMergeTestEntry(5, 1)

		eth := ethExecutablesSnapshot{
			groups: [][]executableTx{
				{senderANonce0, senderANonce1},
				{senderBNonce0, senderBNonce1},
			},
			total: 4,
		}
		actual := mergeExecutables([]executableTx{vechainHigh, vechainLow}, eth)

		assert.Equal(t, transactionsOf(
			vechainHigh,
			senderBNonce0,
			vechainLow,
			senderANonce0,
			senderANonce1,
			senderBNonce1,
		), actual)
	})

	t.Run("newer transaction wins equal priority", func(t *testing.T) {
		older := newMergeTestEntry(10, 1)
		newer := newMergeTestEntry(10, 2)
		actual := mergeExecutables(
			[]executableTx{older},
			ethExecutablesSnapshot{groups: [][]executableTx{{newer}}, total: 1},
		)
		assert.Equal(t, transactionsOf(newer, older), actual)
	})
}

func TestCoordinatorExecutablesMergesFamilies(t *testing.T) {
	vechainEntry := newMergeTestEntry(50, 1)
	ethNonce0 := newMergeTestEntry(10, 1)
	ethNonce1 := newMergeTestEntry(100, 1)

	vechain := &VeChainPool{}
	vechain.executables.Store(&vechainExecutablesSnapshot{
		transactions: transactionsOf(vechainEntry),
		entries:      []executableTx{vechainEntry},
	})

	ethMap := newEthPoolMap(newCostTracker())
	sender := newEthSender(thor.Address{0x01}, 0)
	sender.pending[0] = &TxObject{
		Transaction:      ethNonce0.tx,
		priorityGasPrice: ethNonce0.priorityGasPrice,
		timeAdded:        ethNonce0.timeAdded,
		executable:       true,
	}
	sender.pending[1] = &TxObject{
		Transaction:      ethNonce1.tx,
		priorityGasPrice: ethNonce1.priorityGasPrice,
		timeAdded:        ethNonce1.timeAdded,
		executable:       true,
	}
	ethMap.senders[sender.origin] = sender

	coordinator := &TxPoolCoordinator{
		vechain: vechain,
		eth:     &EthPool{all: ethMap},
	}
	assert.Equal(t, transactionsOf(vechainEntry, ethNonce0, ethNonce1), coordinator.Executables())
}
