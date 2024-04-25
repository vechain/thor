// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestGetByID(t *testing.T) {
	db := muxdb.NewMem()
	repo := newChainRepo(db)

	// Creating transactions
	tx1 := newTx(repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	tx2 := newTx(repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[1])

	// Resolving transactions into txObjects
	txObj1, _ := resolveTx(tx1, false)
	txObj2, _ := resolveTx(tx2, false)

	// Creating a new txObjectMap and adding transactions
	m := newTxObjectMap()
	assert.Nil(t, m.Add(txObj1))
	assert.Nil(t, m.Add(txObj2))

	// Testing GetByID
	retrievedTxObj1 := m.GetByID(txObj1.ID())
	assert.Equal(t, txObj1, retrievedTxObj1, "The retrieved transaction object should match the original for tx1")

	retrievedTxObj2 := m.GetByID(txObj2.ID())
	assert.Equal(t, txObj2, retrievedTxObj2, "The retrieved transaction object should match the original for tx2")

	// Testing retrieval of a non-existing transaction
	nonExistingTxID := thor.Bytes32{} // An arbitrary non-existing ID
	retrievedTxObj3 := m.GetByID(nonExistingTxID)
	assert.Nil(t, retrievedTxObj3, "Retrieving a non-existing transaction should return nil")
}

func TestFill(t *testing.T) {
	db := muxdb.NewMem()
	repo := newChainRepo(db)

	// Creating transactions
	tx1 := newTx(repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	tx2 := newTx(repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[1])

	// Resolving transactions into txObjects
	txObj1, _ := resolveTx(tx1, false)
	txObj2, _ := resolveTx(tx2, false)

	// Creating a new txObjectMap
	m := newTxObjectMap()

	// Filling the map with transactions
	m.Fill([]*txObject{txObj1, txObj2})

	// Asserting the length of the map
	assert.Equal(t, 2, m.Len(), "Map should contain only 2 unique transactions")

	// Asserting the transactions are correctly added
	assert.True(t, m.ContainsHash(txObj1.Hash()), "Map should contain txObj1")
	assert.True(t, m.ContainsHash(txObj2.Hash()), "Map should contain txObj2")

	// Asserting duplicate handling
	assert.Equal(t, m.GetByID(txObj1.ID()), txObj1, "Duplicate tx1 should not be added again")
	assert.Equal(t, m.GetByID(txObj2.ID()), txObj2, "txObj2 should be retrievable by ID")
}

func TestTxObjMap(t *testing.T) {
	db := muxdb.NewMem()
	repo := newChainRepo(db)

	tx1 := newTx(repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	tx2 := newTx(repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	tx3 := newTx(repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[1])

	txObj1, _ := resolveTx(tx1, false)
	txObj2, _ := resolveTx(tx2, false)
	txObj3, _ := resolveTx(tx3, false)

	m := newTxObjectMap()
	assert.Zero(t, m.Len())

	assert.Nil(t, m.Add(txObj1))
	assert.Nil(t, m.Add(txObj1), "should no error if exists")
	assert.Equal(t, 1, m.Len())

	assert.Nil(t, m.Add(txObj2))
	assert.Equal(t, 2, m.Len())

	assert.Nil(t, m.Add(txObj3))
	assert.Equal(t, 3, m.Len())

	assert.True(t, m.ContainsHash(tx1.Hash()))
	assert.True(t, m.ContainsHash(tx2.Hash()))
	assert.True(t, m.ContainsHash(tx3.Hash()))

	assert.True(t, m.RemoveByHash(tx1.Hash()))
	assert.False(t, m.ContainsHash(tx1.Hash()))
	assert.True(t, m.RemoveByHash(tx2.Hash()))

	assert.Equal(t, []*txObject{txObj3}, m.ToTxObjects())
	assert.Equal(t, tx.Transactions{tx3}, m.ToTxs())
}

func TestLimitByDelegator(t *testing.T) {
	db := muxdb.NewMem()
	repo := newChainRepo(db)

	tx1 := newTx(repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	tx2 := newDelegatedTx(repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, genesis.DevAccounts()[0], genesis.DevAccounts()[1])
	tx3 := newDelegatedTx(repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, genesis.DevAccounts()[2], genesis.DevAccounts()[1])

	txObj1, _ := resolveTx(tx1, false)
	txObj2, _ := resolveTx(tx2, false)
	txObj3, _ := resolveTx(tx3, false)

	m := newTxObjectMap()
	assert.Nil(t, m.Add(txObj1))
	assert.Nil(t, m.Add(txObj3))

	m = newTxObjectMap()
	assert.Nil(t, m.Add(txObj2))
	assert.Nil(t, m.Add(txObj2))
	assert.Nil(t, m.Add(txObj3))
}
