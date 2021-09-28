// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"github.com/ethereum/go-ethereum/rlp"
	lru "github.com/hashicorp/golang-lru"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/thor"
)

var codeCache, _ = lru.NewARC(512)

// cachedObject to cache code and storage of an account.
type cachedObject struct {
	db   *muxdb.MuxDB
	data Account

	cache struct {
		code        []byte
		storageTrie *muxdb.Trie
		storage     map[thor.Bytes32]rlp.RawValue
	}
}

func newCachedObject(db *muxdb.MuxDB, data *Account) *cachedObject {
	return &cachedObject{db: db, data: *data}
}

func (co *cachedObject) getOrCreateStorageTrie() *muxdb.Trie {
	if co.cache.storageTrie != nil {
		return co.cache.storageTrie
	}

	trie := co.db.NewSecureTrie(
		StorageTrieName(co.data.meta.Addr, co.data.meta.StorageInitCommitNum),
		thor.BytesToBytes32(co.data.StorageRoot),
		co.data.meta.StorageCommitNum)

	co.cache.storageTrie = trie
	return trie
}

// GetStorage returns storage value for given key.
func (co *cachedObject) GetStorage(key thor.Bytes32, steadyCommitNum uint32) (rlp.RawValue, error) {
	cache := &co.cache
	// retrive from storage cache
	if cache.storage != nil {
		if v, ok := cache.storage[key]; ok {
			return v, nil
		}
	} else {
		cache.storage = make(map[thor.Bytes32]rlp.RawValue)
	}
	// not found in cache

	trie := co.getOrCreateStorageTrie()

	// load from trie
	v, err := loadStorage(trie, key, co.db.TrieLeafBank(), steadyCommitNum)
	if err != nil {
		return nil, err
	}
	// put into cache
	cache.storage[key] = v
	return v, nil
}

// GetCode returns the code of the account.
func (co *cachedObject) GetCode() ([]byte, error) {
	cache := &co.cache

	if len(cache.code) > 0 {
		return cache.code, nil
	}

	if len(co.data.CodeHash) > 0 {
		// do have code
		if code, has := codeCache.Get(string(co.data.CodeHash)); has {
			return code.([]byte), nil
		}

		code, err := co.db.NewStore(codeStoreName).Get(co.data.CodeHash)
		if err != nil {
			return nil, err
		}
		codeCache.Add(string(co.data.CodeHash), code)
		cache.code = code
		return code, nil
	}
	return nil, nil
}
