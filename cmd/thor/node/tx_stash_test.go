// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"bytes"
	"math/rand/v2"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/tx"
)

func newTx(txType int) *tx.Transaction {
	trx, _ := tx.NewTxBuilder(txType).Nonce(rand.Uint64()).Build() //#nosec
	return tx.MustSign(
		trx,
		genesis.DevAccounts()[0].PrivateKey,
	)
}

func TestTxStash(t *testing.T) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	defer db.Close()

	stash := newTxStash(db, 20)

	var saved tx.Transactions
	for i := 0; i < 11; i++ {
		tx := newTx(tx.LegacyTxType)
		assert.Nil(t, stash.Save(tx))
		saved = append(saved, tx)
	}

	for i := 0; i < 11; i++ {
		tx := newTx(tx.DynamicFeeTxType)
		assert.Nil(t, stash.Save(tx))
		saved = append(saved, tx)
	}

	loaded := newTxStash(db, 20).LoadAll()

	saved = saved[2:]
	sort.Slice(saved, func(i, j int) bool {
		return bytes.Compare(saved[i].ID().Bytes(), saved[j].ID().Bytes()) < 0
	})

	sort.Slice(loaded, func(i, j int) bool {
		return bytes.Compare(loaded[i].ID().Bytes(), loaded[j].ID().Bytes()) < 0
	})

	assert.Equal(t, saved.RootHash(), loaded.RootHash())
}
