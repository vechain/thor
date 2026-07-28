// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"errors"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

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
	m := newTxObjectMap()
	assert.Nil(t, m.Add(txObj1, false, nil, 1, func(_ thor.Address, _ *big.Int) error { return nil }))
	assert.Nil(t, m.Add(txObj2, false, nil, 1, func(_ thor.Address, _ *big.Int) error { return nil }))
	assert.Nil(t, m.Add(txObj3, false, nil, 1, func(_ thor.Address, _ *big.Int) error { return nil }))

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
	m := newTxObjectMap()

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

func TestTxObjMap(t *testing.T) {
	repo := newChainRepo()

	tx1 := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	tx2 := newTx(tx.TypeDynamicFee, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	tx3 := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[1])

	txObj1, _ := ResolveTx(tx1, false)
	txObj2, _ := ResolveTx(tx2, false)
	txObj3, _ := ResolveTx(tx3, false)

	m := newTxObjectMap()
	assert.Zero(t, m.Len())

	assert.Nil(t, m.Add(txObj1, false, nil, 1, func(_ thor.Address, _ *big.Int) error { return nil }))
	assert.Nil(t, m.Add(txObj1, false, nil, 1, func(_ thor.Address, _ *big.Int) error { return nil }), "should no error if exists")
	assert.Equal(t, 1, m.Len())

	assert.Equal(t, errors.New("account quota exceeded"), m.Add(txObj2, false, nil, 1, func(_ thor.Address, _ *big.Int) error { return nil }))
	assert.Equal(t, 1, m.Len())

	assert.Nil(t, m.Add(txObj3, false, nil, 1, func(_ thor.Address, _ *big.Int) error { return nil }))
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

	m := newTxObjectMap()
	assert.Nil(t, m.Add(txObj1, false, nil, 1, func(_ thor.Address, _ *big.Int) error { return nil }))
	assert.Nil(t, m.Add(txObj3, false, nil, 1, func(_ thor.Address, _ *big.Int) error { return nil }))

	m = newTxObjectMap()
	assert.Nil(t, m.Add(txObj2, false, nil, 1, func(_ thor.Address, _ *big.Int) error { return nil }))
	assert.Equal(t, errors.New("delegator quota exceeded"), m.Add(txObj3, false, nil, 1, func(_ thor.Address, _ *big.Int) error { return nil }))
	assert.Equal(t, errors.New("account quota exceeded"), m.Add(txObj1, false, nil, 1, func(_ thor.Address, _ *big.Int) error { return nil }))
}

func TestPromoteIfPresentAndRemove(t *testing.T) {
	tchain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	assert.Nil(t, err)
	tchain.MintBlock()
	repo, stater, forkConfig := tchain.Repo(), tchain.Stater(), tchain.GetForkConfig()

	trx := newTx(tx.TypeLegacy, repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	txObj, err := ResolveTx(trx, false)
	assert.Nil(t, err)

	best := repo.BestBlockSummary()
	state := stater.NewState(best.Root())
	baseFee := galactica.CalcBaseFee(best.Header, forkConfig)
	_, pricing, err := txObj.Evaluate(repo.NewBestChain(), state, best.Header, forkConfig, baseFee, false)
	assert.Nil(t, err)
	assert.NotNil(t, pricing)

	m := newTxObjectMap()

	// not in the pool yet: promote returns false and does not account
	txObj.setPricing(pricing)
	assert.False(t, m.promote(txObj))
	assert.Nil(t, m.cost[genesis.DevAccounts()[0].Address])

	// added as non-executable, then promote -> accounted
	assert.Nil(t, m.Add(txObj, false, nil, 10, func(_ thor.Address, _ *big.Int) error { return nil }))
	assert.True(t, m.promote(txObj))
	assert.Equal(t, txObj.Cost(), m.cost[genesis.DevAccounts()[0].Address])

	// idempotent
	assert.True(t, m.promote(txObj))
	assert.Equal(t, txObj.Cost(), m.cost[genesis.DevAccounts()[0].Address])

	// drained to zero after removal
	assert.True(t, m.RemoveByHash(txObj.Hash()))
	assert.Nil(t, m.cost[genesis.DevAccounts()[0].Address])
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
	exec1, p1, err := txObj1.Evaluate(chain, state, best.Header, forkConfig, baseFee, false)
	assert.Nil(t, err)
	assert.True(t, exec1)

	exec2, p2, err := txObj2.Evaluate(chain, state, best.Header, forkConfig, baseFee, false)
	assert.Nil(t, err)
	assert.True(t, exec2)

	exec3, p3, err := txObj3.Evaluate(chain, state, best.Header, forkConfig, baseFee, false)
	assert.Nil(t, err)
	assert.True(t, exec3)

	// Creating a new txObjectMap
	m := newTxObjectMap()

	assert.Nil(t, m.Add(txObj1, true, p1, 10, func(_ thor.Address, _ *big.Int) error { return nil }))
	assert.Nil(t, m.Add(txObj2, true, p2, 10, func(_ thor.Address, _ *big.Int) error { return nil }))
	assert.Nil(t, m.Add(txObj3, true, p3, 10, func(_ thor.Address, _ *big.Int) error { return nil }))

	assert.Equal(t, txObj1.Cost(), m.cost[genesis.DevAccounts()[0].Address])
	// No cost for txObj2's origin, should be counted on the delegator
	assert.Nil(t, m.cost[genesis.DevAccounts()[1].Address])
	assert.Equal(t, new(big.Int).Add(txObj2.Cost(), txObj3.Cost()), m.cost[genesis.DevAccounts()[2].Address])

	m.RemoveByHash(txObj1.Hash())
	assert.Nil(t, m.cost[genesis.DevAccounts()[0].Address])
	m.RemoveByHash(txObj2.Hash())
	assert.Equal(t, txObj3.Cost(), m.cost[genesis.DevAccounts()[2].Address])
	m.RemoveByHash(txObj2.Hash())
	assert.Equal(t, txObj3.Cost(), m.cost[genesis.DevAccounts()[2].Address])
	m.RemoveByHash(txObj3.Hash())
	assert.Nil(t, m.cost[genesis.DevAccounts()[2].Address])
}
