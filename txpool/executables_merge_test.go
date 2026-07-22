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

func TestMergePoolExecutables(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		assert.Nil(t, mergePoolExecutables(nil, ethExecutablesSnapshot{}))
	})

	t.Run("one source", func(t *testing.T) {
		first := newMergeTestEntry(20, 1)
		second := newMergeTestEntry(10, 1)
		assert.Equal(t, transactionsOf(first, second), mergePoolExecutables(
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
		actual := mergePoolExecutables([]executableTx{vechainHigh, vechainLow}, eth)

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
		actual := mergePoolExecutables(
			[]executableTx{older},
			ethExecutablesSnapshot{groups: [][]executableTx{{newer}}, total: 1},
		)
		assert.Equal(t, transactionsOf(newer, older), actual)
	})
}

func TestMergePoolExecutablesManySourcesAndEmptyGroups(t *testing.T) {
	const senderCount = 64
	groups := make([][]executableTx, 0, senderCount*2)
	expectedEntries := make([]executableTx, 0, senderCount)
	for i := range senderCount {
		entry := newMergeTestEntry(int64(senderCount-i), int64(i))
		expectedEntries = append(expectedEntries, entry)
		groups = append(groups, nil, []executableTx{entry})
	}

	actual := mergePoolExecutables(nil, ethExecutablesSnapshot{
		groups: groups,
		total:  senderCount,
	})

	assert.Equal(t, transactionsOf(expectedEntries...), actual)

	vechainOnly := []executableTx{
		newMergeTestEntry(20, 2),
		newMergeTestEntry(10, 1),
	}
	assert.Equal(t, transactionsOf(vechainOnly...), mergePoolExecutables(
		vechainOnly,
		ethExecutablesSnapshot{groups: [][]executableTx{nil, {}}, total: 0},
	))
}

func TestMergePoolExecutablesExactTieUsesSourceOrder(t *testing.T) {
	vechain := newMergeTestEntry(10, 5)
	firstEthSender := newMergeTestEntry(10, 5)
	secondEthSender := newMergeTestEntry(10, 5)
	eth := ethExecutablesSnapshot{
		groups: [][]executableTx{
			{firstEthSender},
			{},
			{secondEthSender},
		},
		total: 2,
	}

	for range 10 {
		assert.Equal(t, transactionsOf(
			vechain,
			firstEthSender,
			secondEthSender,
		), mergePoolExecutables([]executableTx{vechain}, eth))
	}
}

func TestEthPoolExecutables(t *testing.T) {
	ethMap := newEthPoolMap(newCostTracker())
	pool := &EthPool{all: ethMap}
	assert.Nil(t, pool.Executables())

	senderANonce0 := newMergeTestEntry(10, 1)
	senderANonce1 := newMergeTestEntry(1_000, 1)
	senderBNonce0 := newMergeTestEntry(50, 1)

	senderA := newEthSender(thor.Address{0x01}, 0)
	senderA.pending[0] = &TxObject{
		Transaction:      senderANonce0.tx,
		priorityGasPrice: senderANonce0.priorityGasPrice,
		timeAdded:        senderANonce0.timeAdded,
		executable:       true,
	}
	senderA.pending[1] = &TxObject{
		Transaction:      senderANonce1.tx,
		priorityGasPrice: senderANonce1.priorityGasPrice,
		timeAdded:        senderANonce1.timeAdded,
		executable:       true,
	}
	senderB := newEthSender(thor.Address{0x02}, 0)
	senderB.pending[0] = &TxObject{
		Transaction:      senderBNonce0.tx,
		priorityGasPrice: senderBNonce0.priorityGasPrice,
		timeAdded:        senderBNonce0.timeAdded,
		executable:       true,
	}
	ethMap.senders[senderA.origin] = senderA
	ethMap.senders[senderB.origin] = senderB

	assert.Equal(t, transactionsOf(
		senderBNonce0,
		senderANonce0,
		senderANonce1,
	), pool.Executables())
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
