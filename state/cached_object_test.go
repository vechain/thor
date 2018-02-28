package state

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/thor"
)

func TestCachedObject(t *testing.T) {
	kv, _ := lvldb.NewMem()

	db := newTrieDatabase(kv)
	stgTrie, _ := trie.NewSecure(common.Hash{}, db, 0)
	storages := []struct {
		k thor.Hash
		v []byte
	}{
		{thor.BytesToHash([]byte("key1")), []byte("value1")},
		{thor.BytesToHash([]byte("key2")), []byte("value2")},
		{thor.BytesToHash([]byte("key3")), []byte("value3")},
		{thor.BytesToHash([]byte("key4")), []byte("value4")},
	}

	for _, s := range storages {
		saveStorage(stgTrie, s.k, s.v)
	}

	storageRoot, _ := stgTrie.Commit(nil)
	db.Commit(storageRoot, false)

	code := make([]byte, 100)
	rand.Read(code)

	codeHash := crypto.Keccak256(code)
	kv.Put(codeHash, code)

	account := Account{
		Balance:     &big.Int{},
		CodeHash:    codeHash,
		StorageRoot: storageRoot[:],
	}
	db = newTrieDatabase(kv)
	obj := newCachedObject(db, account)

	assert.Equal(t,
		M(obj.GetCode()),
		[]interface{}{code, nil})

	for _, s := range storages {
		assert.Equal(t,
			M(obj.GetStorage(s.k)),
			[]interface{}{s.v, nil})
	}
}
