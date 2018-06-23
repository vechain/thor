// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/thor"
)

// cachedObject to cache code and storage of an account.
type cachedObject struct {
	kv   kv.GetPutter
	data Account

	cache struct {
		code        []byte
		storageTrie trieReader
		storage     map[thor.Bytes32]rlp.RawValue
	}
}

func newCachedObject(kv kv.GetPutter, data *Account) *cachedObject {
	return &cachedObject{kv: kv, data: *data}
}

func (co *cachedObject) getOrCreateStorageTrie() (trieReader, error) {
	if co.cache.storageTrie != nil {
		return co.cache.storageTrie, nil
	}

	root := thor.BytesToBytes32(co.data.StorageRoot)

	trie, err := trCache.Get(root, co.kv, false)
	if err != nil {
		return nil, err
	}
	co.cache.storageTrie = trie
	return trie, nil
}

// GetStorage returns storage value for given key.
func (co *cachedObject) GetStorage(key thor.Bytes32) (rlp.RawValue, error) {
	cache := &co.cache
	// retrive from storage cache
	if cache.storage == nil {
		cache.storage = make(map[thor.Bytes32]rlp.RawValue)
	} else {
		if v, ok := cache.storage[key]; ok {
			return v, nil
		}
	}
	// not found in cache

	trie, err := co.getOrCreateStorageTrie()
	if err != nil {
		return nil, err
	}

	// load from trie
	v, err := loadStorage(trie, key)
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
		code, err := co.kv.Get(co.data.CodeHash)
		if err != nil {
			return nil, err
		}
		cache.code = code
		return code, nil
	}
	return nil, nil
}
