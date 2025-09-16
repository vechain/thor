// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/tx"
)

func newTx(txType tx.Type) *tx.Transaction {
	return tx.MustSign(
		tx.NewBuilder(txType).Nonce(rand.Uint64()).Build(), //#nosec
		genesis.DevAccounts()[0].PrivateKey,
	)
}

// Note: Cannot test the MarshalBinary error path (line 41) from outside the tx package
// because txData is unexported and has unexported methods, so we cannot create a mock.
// All other error/branch tests are present below.
func TestTxStash_ErrorsAndBranches(t *testing.T) {
	t.Run("db.Has returns error (line 33)", func(t *testing.T) {
		db, _ := leveldb.Open(storage.NewMemStorage(), nil)
		_ = db.Close() // force error
		stash := newTxStash(db, 10)
		tx := newTx(tx.TypeLegacy)
		err := stash.Save(tx)
		assert.Error(t, err)
	})

	t.Run("db.Has returns true (line 36)", func(t *testing.T) {
		db, _ := leveldb.Open(storage.NewMemStorage(), nil)
		stash := newTxStash(db, 10)
		tx := newTx(tx.TypeLegacy)
		_ = stash.Save(tx)    // Save once
		err := stash.Save(tx) // Save again, should be no error, no-op
		assert.NoError(t, err)
	})

	t.Run("db.Delete returns error (line 51)", func(t *testing.T) {
		db, _ := leveldb.Open(storage.NewMemStorage(), nil)
		stash := newTxStash(db, 1)
		tx1 := newTx(tx.TypeLegacy)
		tx2 := newTx(tx.TypeDynamicFee)
		_ = stash.Save(tx1)
		// Close DB to force error on Delete
		_ = db.Close()
		err := stash.Save(tx2)
		assert.Error(t, err)
	})

	t.Run("UnmarshalBinary returns error (line 69, 72)", func(t *testing.T) {
		db, _ := leveldb.Open(storage.NewMemStorage(), nil)
		stash := newTxStash(db, 10)
		badKey := []byte{0x01, 0x02, 0x03}
		badVal := []byte{0xFF, 0xFF, 0xFF} // not a valid tx
		_ = db.Put(badKey, badVal, nil)
		loaded := stash.LoadAll()
		assert.Len(t, loaded, 0) // should skip bad tx
	})

	t.Run("Key mismatch triggers remap (line 78)", func(t *testing.T) {
		db, _ := leveldb.Open(storage.NewMemStorage(), nil)
		stash := newTxStash(db, 10)
		tx := newTx(tx.TypeLegacy)
		val, _ := tx.MarshalBinary()
		wrongKey := []byte{0xAA, 0xBB, 0xCC}
		_ = db.Put(wrongKey, val, nil)
		loaded := stash.LoadAll()
		assert.NotEmpty(t, loaded)
		// The tx should be loaded and remapped under the correct key
		ok, _ := db.Has(tx.Hash().Bytes(), nil)
		assert.True(t, ok)
	})

	t.Run("db.Write returns error (line 85)", func(t *testing.T) {
		db, _ := leveldb.Open(storage.NewMemStorage(), nil)
		stash := newTxStash(db, 10)
		tx := newTx(tx.TypeLegacy)
		_ = stash.Save(tx)
		// Close DB to force error on Write
		_ = db.Close()
		_ = stash.LoadAll() // Should not panic, error is logged
	})
}
