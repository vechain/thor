// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/tx"
)

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

	assert.Nil(t, m.Add(txObj1, 1))
	assert.Nil(t, m.Add(txObj1, 1), "should no error if exists")
	assert.Equal(t, 1, m.Len())

	assert.Equal(t, errors.New("account quota exceeded"), m.Add(txObj2, 1))
	assert.Equal(t, 1, m.Len())

	assert.Nil(t, m.Add(txObj3, 1))
	assert.Equal(t, 2, m.Len())

	assert.True(t, m.ContainsHash(tx1.Hash()))
	assert.False(t, m.ContainsHash(tx2.Hash()))
	assert.True(t, m.ContainsHash(tx3.Hash()))

	assert.True(t, m.RemoveByHash(tx1.Hash()))
	assert.False(t, m.ContainsHash(tx1.Hash()))
	assert.False(t, m.RemoveByHash(tx2.Hash()))

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
	assert.Nil(t, m.Add(txObj1, 1))
	assert.Nil(t, m.Add(txObj3, 1))

	m = newTxObjectMap()
	assert.Nil(t, m.Add(txObj2, 1))
	assert.Equal(t, errors.New("delegator quota exceeded"), m.Add(txObj3, 1))
	assert.Equal(t, errors.New("account quota exceeded"), m.Add(txObj1, 1))
}
