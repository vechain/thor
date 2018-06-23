// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

func TestCachedObject(t *testing.T) {
	kv, _ := lvldb.NewMem()

	stgTrie, _ := trie.NewSecure(thor.Bytes32{}, kv, 0)
	storages := []struct {
		k thor.Bytes32
		v rlp.RawValue
	}{
		{thor.BytesToBytes32([]byte("key1")), []byte("value1")},
		{thor.BytesToBytes32([]byte("key2")), []byte("value2")},
		{thor.BytesToBytes32([]byte("key3")), []byte("value3")},
		{thor.BytesToBytes32([]byte("key4")), []byte("value4")},
	}

	for _, s := range storages {
		saveStorage(stgTrie, s.k, s.v)
	}

	storageRoot, _ := stgTrie.Commit()

	code := make([]byte, 100)
	rand.Read(code)

	codeHash := crypto.Keccak256(code)
	kv.Put(codeHash, code)

	account := Account{
		Balance:     &big.Int{},
		CodeHash:    codeHash,
		StorageRoot: storageRoot[:],
	}

	obj := newCachedObject(kv, &account)

	assert.Equal(t,
		M(obj.GetCode()),
		[]interface{}{code, nil})

	for _, s := range storages {
		assert.Equal(t,
			M(obj.GetStorage(s.k)),
			[]interface{}{s.v, nil})
	}
}
