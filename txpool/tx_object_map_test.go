// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"errors"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/consensus/upgrade/galactica"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestGetByID(t *testing.T) {
	repo := newChainRepo()

	// Creating transactions
	tx1 := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	tx2 := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[1])
	tx3 := newTx(tx.TypeDynamicFee, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[2])

	// Resolving transactions into txObjects
	txObj1, _ := ResolveTx(tx1, false)
	txObj2, _ := ResolveTx(tx2, false)
	txObj3, _ := ResolveTx(tx3, false)

	// Creating a new txObjectMap and adding transactions
	m := newTxObjectMap(newCostTracker())
	assert.Nil(t, m.Add(txObj1, 1, nil))
	assert.Nil(t, m.Add(txObj2, 1, nil))
	assert.Nil(t, m.Add(txObj3, 1, nil))

	// Testing GetByID
	retrievedTxObj1 := m.GetByID(txObj1.ID())
	assert.Equal(t, txObj1, retrievedTxObj1, "The retrieved transaction object should match the original for tx1")

	retrievedTxObj2 := m.GetByID(txObj2.ID())
	assert.Equal(t, txObj2, retrievedTxObj2, "The retrieved transaction object should match the original for tx2")

	retrievedTxObj3 := m.GetByID(txObj3.ID())
	assert.Equal(t, txObj3, retrievedTxObj3, "The retrieved transaction object should match the original for tx3")

	// Testing retrieval of a non-existing transaction
	nonExistingTxID := thor.Bytes32{} // An arbitrary non-existing ID
	retrievedNonExistingTxObj3 := m.GetByID(nonExistingTxID)
	assert.Nil(t, retrievedNonExistingTxObj3, "Retrieving a non-existing transaction should return nil")
}

func TestFill(t *testing.T) {
	repo := newChainRepo()

	// Creating transactions
	tx1 := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	tx2 := newDelegatedTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, genesis.DevAccounts()[1], genesis.DevAccounts()[2])
	tx3 := newDelegatedTx(tx.TypeDynamicFee, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, genesis.DevAccounts()[3], genesis.DevAccounts()[4])

	// Resolving transactions into txObjects
	txObj1, _ := ResolveTx(tx1, false)
	txObj2, _ := ResolveTx(tx2, false)
	txObj3, _ := ResolveTx(tx3, false)

	// Creating a new txObjectMap
	m := newTxObjectMap(newCostTracker())

	// Filling the map with transactions
	m.Fill([]*TxObject{txObj1, txObj2, txObj1, txObj3})

	// Asserting the length of the map
	assert.Equal(t, 3, m.Len(), "Map should contain only 2 unique transactions")

	// Asserting the transactions are correctly added
	assert.True(t, m.ContainsHash(txObj1.Hash()), "Map should contain txObj1")
	assert.True(t, m.ContainsHash(txObj2.Hash()), "Map should contain txObj2")
	assert.True(t, m.ContainsHash(txObj3.Hash()), "Map should contain txObj3")

	// Asserting duplicate handling
	assert.Equal(t, m.GetByID(txObj1.ID()), txObj1, "Duplicate tx1 should not be added again")
	assert.Equal(t, m.GetByID(txObj2.ID()), txObj2, "txObj2 should be retrievable by ID")
	assert.Equal(t, m.GetByID(txObj3.ID()), txObj3, "txObj3 should be retrievable by ID")

	assert.Equal(t, 1, m.quota[genesis.DevAccounts()[0].Address], "Account quota should be 1 for account 0")
	assert.Equal(t, 1, m.quota[genesis.DevAccounts()[1].Address], "Account quota should be 1 for account 1")
	assert.Equal(t, 1, m.quota[genesis.DevAccounts()[2].Address], "Delegator quota should be 1 for account 2")
	assert.Equal(t, 1, m.quota[genesis.DevAccounts()[3].Address], "Account quota should be 1 for account 3")
	assert.Equal(t, 1, m.quota[genesis.DevAccounts()[4].Address], "Delegator quota should be 1 for account 4")
}

func TestTxObjectMapCountsTrackMutations(t *testing.T) {
	repo := newChainRepo()
	nonExecutableTx := newTx(
		tx.TypeLegacy,
		repo.ChainTag(),
		nil,
		21_000,
		tx.BlockRef{},
		100,
		nil,
		tx.Features(0),
		genesis.DevAccounts()[0],
	)
	executableTx := newTx(
		tx.TypeLegacy,
		repo.ChainTag(),
		nil,
		21_000,
		tx.BlockRef{},
		101,
		nil,
		tx.Features(0),
		genesis.DevAccounts()[1],
	)
	filledTx := newTx(
		tx.TypeLegacy,
		repo.ChainTag(),
		nil,
		21_000,
		tx.BlockRef{},
		102,
		nil,
		tx.Features(0),
		genesis.DevAccounts()[2],
	)

	nonExecutable, err := ResolveTx(nonExecutableTx, false)
	require.NoError(t, err)
	executable, err := ResolveTx(executableTx, false)
	require.NoError(t, err)
	executable.executable = true
	filled, err := ResolveTx(filledTx, false)
	require.NoError(t, err)
	filled.executable = true

	m := newTxObjectMap(newCostTracker())
	require.NoError(t, m.Add(nonExecutable, 10, nil))
	require.NoError(t, m.Add(executable, 10, nil))
	require.NoError(t, m.Add(executable, 10, nil))
	total, executableCount := m.Counts()
	assert.Equal(t, 2, total)
	assert.Equal(t, 1, executableCount)

	require.True(t, m.RemoveByHash(executable.Hash()))
	total, executableCount = m.Counts()
	assert.Equal(t, 1, total)
	assert.Zero(t, executableCount)

	payer := nonExecutable.Origin()
	nonExecutable.payer = &payer
	nonExecutable.cost = big.NewInt(1)
	reserved, err := m.ReserveCost(nonExecutable, big.NewInt(1))
	require.NoError(t, err)
	require.True(t, reserved)
	total, executableCount = m.Counts()
	assert.Equal(t, 1, total)
	assert.Equal(t, 1, executableCount)

	m.Fill([]*TxObject{filled, filled})
	total, executableCount = m.Counts()
	assert.Equal(t, 2, total)
	assert.Equal(t, 2, executableCount)
}

func TestTxObjectMapCountsInvariantUnderMixedMutations(t *testing.T) {
	repo := newChainRepo()
	txObjs := make([]*TxObject, 32)
	for i := range txObjs {
		trx := newTx(
			tx.TypeLegacy,
			repo.ChainTag(),
			nil,
			21_000,
			tx.BlockRef{},
			uint32(100+i),
			nil,
			tx.Features(0),
			genesis.DevAccounts()[i%len(genesis.DevAccounts())],
		)
		txObj, err := ResolveTx(trx, false)
		require.NoError(t, err)
		txObjs[i] = txObj
	}

	m := newTxObjectMap(newCostTracker())
	balance := big.NewInt(1_000_000)
	var sequence uint64 = 1
	for range 5_000 {
		sequence = sequence*6364136223846793005 + 1442695040888963407
		index := int(sequence % uint64(len(txObjs)))
		txObj := txObjs[index]

		switch (sequence >> 32) % 6 {
		case 0:
			_ = m.Add(txObj, len(txObjs), balance)
		case 1:
			m.RemoveByHash(txObj.Hash())
		case 2:
			m.Fill([]*TxObject{txObj, txObj})
		case 3:
			if m.GetByHash(txObj.Hash()) == txObj {
				payer := txObj.Origin()
				txObj.payer = &payer
				txObj.cost = big.NewInt(1)
				_, _ = m.ReserveCost(txObj, balance)
			}
		case 4:
			other := txObjs[(index+1)%len(txObjs)]
			m.RemoveByHashAndID(txObj.Hash(), other.ID())
		case 5:
			m.RemoveByHashAndID(txObj.Hash(), txObj.ID())
		}

		assertTxObjectMapCountsInvariant(t, m)
	}
}

func assertTxObjectMapCountsInvariant(t *testing.T, m *txObjectMap) {
	t.Helper()

	m.lock.RLock()
	defer m.lock.RUnlock()

	executableCount := 0
	for _, txObj := range m.mapByHash {
		if txObj.executable {
			executableCount++
		}
	}
	assert.Equal(t, len(m.mapByHash), len(m.mapByID))
	assert.Equal(t, executableCount, m.executableCount)
	assert.GreaterOrEqual(t, m.executableCount, 0)
	assert.LessOrEqual(t, m.executableCount, len(m.mapByHash))
}

func TestTxObjMap(t *testing.T) {
	repo := newChainRepo()

	tx1 := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	tx2 := newTx(tx.TypeDynamicFee, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	tx3 := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[1])

	txObj1, _ := ResolveTx(tx1, false)
	txObj2, _ := ResolveTx(tx2, false)
	txObj3, _ := ResolveTx(tx3, false)

	m := newTxObjectMap(newCostTracker())
	assert.Zero(t, m.Len())

	assert.Nil(t, m.Add(txObj1, 1, nil))
	assert.Nil(t, m.Add(txObj1, 1, nil), "should no error if exists")
	assert.Equal(t, 1, m.Len())

	assert.Equal(t, errors.New("account quota exceeded"), m.Add(txObj2, 1, nil))
	assert.Equal(t, 1, m.Len())

	assert.Nil(t, m.Add(txObj3, 1, nil))
	assert.Equal(t, 2, m.Len())

	assert.True(t, m.ContainsHash(tx1.Hash()))
	assert.False(t, m.ContainsHash(tx2.Hash()))
	assert.True(t, m.ContainsHash(tx3.Hash()))

	assert.True(t, m.RemoveByHash(tx1.Hash()))
	assert.False(t, m.ContainsHash(tx1.Hash()))
	assert.False(t, m.RemoveByHash(tx2.Hash()))

	assert.Equal(t, []*TxObject{txObj3}, m.ToTxObjects())
	assert.Equal(t, tx.Transactions{tx3}, m.ToTxs())
}

func TestLimitByDelegator(t *testing.T) {
	repo := newChainRepo()

	tx1 := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	tx2 := newDelegatedTx(tx.TypeDynamicFee, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, genesis.DevAccounts()[0], genesis.DevAccounts()[1])
	tx3 := newDelegatedTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, genesis.DevAccounts()[2], genesis.DevAccounts()[1])

	txObj1, _ := ResolveTx(tx1, false)
	txObj2, _ := ResolveTx(tx2, false)
	txObj3, _ := ResolveTx(tx3, false)

	m := newTxObjectMap(newCostTracker())
	assert.Nil(t, m.Add(txObj1, 1, nil))
	assert.Nil(t, m.Add(txObj3, 1, nil))

	m = newTxObjectMap(newCostTracker())
	assert.Nil(t, m.Add(txObj2, 1, nil))
	assert.Equal(t, errors.New("delegator quota exceeded"), m.Add(txObj3, 1, nil))
	assert.Equal(t, errors.New("account quota exceeded"), m.Add(txObj1, 1, nil))
}

func TestPendingCost(t *testing.T) {
	tchain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	assert.Nil(t, err)

	repo := tchain.Repo()
	stater := tchain.Stater()
	forkConfig := tchain.GetForkConfig()

	tchain.MintBlock()

	// Creating transactions
	tx1 := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	tx2 := newDelegatedTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, genesis.DevAccounts()[1], genesis.DevAccounts()[2])
	tx3 := newDelegatedTx(tx.TypeDynamicFee, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, genesis.DevAccounts()[1], genesis.DevAccounts()[2])

	// Resolving transactions into txObjects
	txObj1, _ := ResolveTx(tx1, false)
	txObj2, _ := ResolveTx(tx2, false)
	txObj3, _ := ResolveTx(tx3, false)

	chain := repo.NewBestChain()
	best := repo.BestBlockSummary()
	state := stater.NewState(best.Root())

	baseFee := galactica.CalcBaseFee(best.Header, forkConfig)
	txObj1.executable, err = txObj1.Executable(chain, state, best.Header, forkConfig, baseFee)
	assert.Nil(t, err)
	assert.True(t, txObj1.executable)

	txObj2.executable, err = txObj2.Executable(chain, state, best.Header, forkConfig, baseFee)
	assert.Nil(t, err)
	assert.True(t, txObj2.executable)

	txObj3.executable, err = txObj3.Executable(chain, state, best.Header, forkConfig, baseFee)
	assert.Nil(t, err)
	assert.True(t, txObj3.executable)

	// Creating a new txObjectMap
	m := newTxObjectMap(newCostTracker())
	balance := new(big.Int).Lsh(big.NewInt(1), 256)

	m.Add(txObj1, 10, balance)
	m.Add(txObj2, 10, balance)
	m.Add(txObj3, 10, balance)

	assert.Equal(t, txObj1.Cost(), m.costs.pendingCost(genesis.DevAccounts()[0].Address))
	// No cost for txObj2's origin, should be counted on the delegator
	assert.Equal(t, 0, m.costs.pendingCost(genesis.DevAccounts()[1].Address).Sign())
	assert.Equal(t, new(big.Int).Add(txObj2.Cost(), txObj3.Cost()), m.costs.pendingCost(genesis.DevAccounts()[2].Address))

	m.RemoveByHash(txObj1.Hash())
	assert.Equal(t, 0, m.costs.pendingCost(genesis.DevAccounts()[0].Address).Sign())
	m.RemoveByHash(txObj2.Hash())
	assert.Equal(t, txObj3.Cost(), m.costs.pendingCost(genesis.DevAccounts()[2].Address))
	m.RemoveByHash(txObj2.Hash())
	assert.Equal(t, txObj3.Cost(), m.costs.pendingCost(genesis.DevAccounts()[2].Address))
	m.RemoveByHash(txObj3.Hash())
	assert.Equal(t, 0, m.costs.pendingCost(genesis.DevAccounts()[2].Address).Sign())
}

func TestTxObjectMapReserveCostAfterRemoval(t *testing.T) {
	repo := newChainRepo()
	trx := newTx(
		tx.TypeDynamicFee,
		repo.ChainTag(),
		nil,
		21_000,
		tx.BlockRef{},
		100,
		nil,
		tx.Features(0),
		genesis.DevAccounts()[0],
	)
	txObj, err := ResolveTx(trx, false)
	assert.NoError(t, err)

	costs := newCostTracker()
	m := newTxObjectMap(costs)
	assert.NoError(t, m.Add(txObj, 1, nil))
	assert.True(t, m.RemoveByHash(txObj.Hash()))

	payer := txObj.Origin()
	txObj.payer = &payer
	txObj.cost = big.NewInt(10)
	reserved, err := m.ReserveCost(txObj, big.NewInt(100))

	assert.NoError(t, err)
	assert.False(t, reserved)
	assert.Zero(t, costs.pendingCost(payer).Sign())
	assert.Empty(t, costs.reservations)
}

func TestTxObjectMapExecutableSnapshotSkipsMissingPriority(t *testing.T) {
	repo := newChainRepo()
	trx := newTx(
		tx.TypeDynamicFee,
		repo.ChainTag(),
		nil,
		21_000,
		tx.BlockRef{},
		100,
		nil,
		tx.Features(0),
		genesis.DevAccounts()[0],
	)
	txObj, err := ResolveTx(trx, false)
	assert.NoError(t, err)

	m := newTxObjectMap(newCostTracker())
	assert.NoError(t, m.Add(txObj, 1, nil))

	snapshot := m.executableSnapshot(tx.Transactions{trx})
	assert.Empty(t, snapshot.transactions())
	assert.Empty(t, *snapshot)

	txObj.priorityGasPrice = big.NewInt(1)
	snapshot = m.executableSnapshot(tx.Transactions{trx})
	assert.Equal(t, tx.Transactions{trx}, snapshot.transactions())
	assert.Len(t, *snapshot, 1)
	assert.Same(t, trx, (*snapshot)[0].tx)
}
