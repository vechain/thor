// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/vechain/thor/kv"
)

// bucket uses prefix to simulate bucket.
type bucket []byte

func (b bucket) ProxyGetter(getter kv.Getter) kv.Getter {
	return &struct {
		kv.GetFunc
		kv.HasFunc
	}{
		func(key []byte) ([]byte, error) {
			return getter.Get(b.makeKey(key))
		},
		func(key []byte) (bool, error) {
			return getter.Has(b.makeKey(key))
		},
	}
}

func (b bucket) ProxyPutter(putter kv.Putter) kv.Putter {
	return &struct {
		kv.PutFunc
		kv.DeleteFunc
	}{
		func(key, val []byte) error {
			return putter.Put(b.makeKey(key), val)
		},
		func(key []byte) error {
			return putter.Delete(b.makeKey(key))
		},
	}
}

func (b bucket) MakeRange(r kv.Range) kv.Range {
	r.Start = b.makeKey(r.Start)
	r.Limit = util.BytesPrefix(b.makeKey(r.Limit)).Limit
	return r
}

func (b bucket) MakePair(pair kv.Pair) kv.Pair {
	return &struct {
		kv.KeyFunc
		kv.ValueFunc
	}{
		func() []byte {
			// skip bucket prefix
			return pair.Key()[len(b):]
		},
		pair.Value,
	}
}

func (b bucket) makeKey(key []byte) []byte {
	newKey := make([]byte, 0, len(b)+len(key))
	return append(append(newKey, b...), key...)
}
