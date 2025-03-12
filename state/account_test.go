// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func M(a ...any) []any {
	return a
}

func TestAccount(t *testing.T) {
	assert.True(t, emptyAccount().IsEmpty())

	acc := emptyAccount()
	acc.Balance = big.NewInt(1)
	assert.False(t, acc.IsEmpty())
	acc = emptyAccount()
	acc.CodeHash = []byte{1}
	assert.False(t, acc.IsEmpty())

	acc = emptyAccount()
	acc.Energy = big.NewInt(1)
	assert.False(t, acc.IsEmpty())

	acc = emptyAccount()
	acc.StorageRoot = []byte{1}
	assert.True(t, acc.IsEmpty())
}

func TestTrie(t *testing.T) {
	db := muxdb.NewMem()
	tr := db.NewTrie("", trie.Root{})

	addr := thor.BytesToAddress([]byte("account1"))
	assert.Equal(t,
		M(loadAccount(tr, addr)),
		M(emptyAccount(), &AccountMetadata{}, nil),
		"should load an empty account")

	acc1 := Account{
		big.NewInt(1),
		big.NewInt(0),
		0,
		[]byte("master"),
		[]byte("code hash"),
		[]byte("storage root"),
	}
	meta1 := AccountMetadata{
		StorageID:       []byte("sid"),
		StorageMajorVer: 1,
		StorageMinorVer: 2,
	}
	saveAccount(tr, addr, &acc1, &meta1)
	assert.Equal(t,
		M(loadAccount(tr, addr)),
		M(&acc1, &meta1, nil))

	saveAccount(tr, addr, emptyAccount(), &meta1)
	assert.Equal(t,
		M(tr.Get(addr[:])),
		M([]byte(nil), []byte(nil), nil),
		"empty account should be deleted")
}

func TestStorageTrie(t *testing.T) {
	db := muxdb.NewMem()
	tr := db.NewTrie("", trie.Root{})

	key := thor.BytesToBytes32([]byte("key"))
	assert.Equal(t,
		M(loadStorage(tr, key)),
		M(rlp.RawValue(nil), nil))

	value := rlp.RawValue("value")
	saveStorage(tr, key, value)
	assert.Equal(t,
		M(loadStorage(tr, key)),
		M(value, nil))

	saveStorage(tr, key, nil)
	assert.Equal(t,
		M(tr.Get(key[:])),
		M([]byte(nil), []byte(nil), nil),
		"empty storage value should be deleted")
}
