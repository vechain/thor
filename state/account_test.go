// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/thor"
)

func M(a ...interface{}) []interface{} {
	return a
}

func TestAccount(t *testing.T) {
	assert.True(t, emptyAccount(thor.Address{}).IsEmpty())

	acc := emptyAccount(thor.Address{})
	acc.Balance = big.NewInt(1)
	assert.False(t, acc.IsEmpty())
	acc = emptyAccount(thor.Address{})
	acc.CodeHash = []byte{1}
	assert.False(t, acc.IsEmpty())

	acc = emptyAccount(thor.Address{})
	acc.Energy = big.NewInt(1)
	assert.False(t, acc.IsEmpty())

	acc = emptyAccount(thor.Address{})
	acc.StorageRoot = []byte{1}
	assert.True(t, acc.IsEmpty())
}

func newTrie() *muxdb.Trie {
	return muxdb.NewMem().NewSecureTrie("", thor.Bytes32{}, 0)
}
func TestTrie(t *testing.T) {
	db := muxdb.NewMem()
	trie := db.NewSecureTrie("", thor.Bytes32{}, 0)

	addr := thor.BytesToAddress([]byte("account1"))
	assert.Equal(t,
		M(loadAccount(trie, addr, db.TrieLeafBank(), 0)),
		M(emptyAccount(addr), nil),
		"should load an empty account")

	acc1 := Account{
		big.NewInt(1),
		big.NewInt(0),
		0,
		[]byte("master"),
		[]byte("code hash"),
		[]byte("storage root"),
		AccountMetadata{
			Addr: addr,
		},
	}
	saveAccount(trie, &acc1)
	assert.Equal(t,
		M(loadAccount(trie, addr, db.TrieLeafBank(), 0)),
		M(&acc1, nil))

	saveAccount(trie, emptyAccount(addr))
	assert.Equal(t,
		M(trie.Get(addr[:])),
		M([]byte(nil), []byte(nil), nil),
		"empty account should be deleted")
}

func TestStorageTrie(t *testing.T) {
	db := muxdb.NewMem()
	trie := db.NewSecureTrie("", thor.Bytes32{}, 0)

	key := thor.BytesToBytes32([]byte("key"))
	assert.Equal(t,
		M(loadStorage(trie, key, db.TrieLeafBank(), 0)),
		M(rlp.RawValue(nil), nil))

	value := rlp.RawValue("value")
	saveStorage(trie, key, value)
	assert.Equal(t,
		M(loadStorage(trie, key, db.TrieLeafBank(), 0)),
		M(value, nil))

	saveStorage(trie, key, nil)
	assert.Equal(t,
		M(trie.Get(key[:])),
		M([]byte(nil), []byte(nil), nil),
		"empty storage value should be deleted")
}
