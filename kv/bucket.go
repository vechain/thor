// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package kv

import (
	"sync"

	"github.com/syndtr/goleveldb/leveldb/util"
)

// Bucket provides logical bucket for kv store.
type Bucket string

// NewGetter creates a bucket getter from the source getter.
func (b Bucket) NewGetter(src Getter) Getter {
	return &struct {
		GetFunc
		HasFunc
		IsNotFoundFunc
	}{
		func(key []byte) ([]byte, error) {
			buf := bufPool.Get().(*buf)
			defer bufPool.Put(buf)
			buf.k = append(append(buf.k[:0], b...), key...)

			return src.Get(buf.k)
		},
		func(key []byte) (bool, error) {
			buf := bufPool.Get().(*buf)
			defer bufPool.Put(buf)
			buf.k = append(append(buf.k[:0], b...), key...)

			return src.Has(buf.k)
		},
		src.IsNotFound,
	}
}

// NewPutter creates a bucket putter from the source putter.
func (b Bucket) NewPutter(src Putter) Putter {
	return &struct {
		PutFunc
		DeleteFunc
	}{
		func(key, val []byte) error {
			buf := bufPool.Get().(*buf)
			defer bufPool.Put(buf)
			buf.k = append(append(buf.k[:0], b...), key...)

			return src.Put(buf.k, val)
		},
		func(key []byte) error {
			buf := bufPool.Get().(*buf)
			defer bufPool.Put(buf)
			buf.k = append(append(buf.k[:0], b...), key...)

			return src.Delete(buf.k)
		},
	}
}

// NewStore creates a bucket store from the source store.
func (b Bucket) NewStore(src Store) Store {
	return &struct {
		Getter
		Putter
		SnapshotFunc
		BatchFunc
		IterateFunc
	}{
		b.NewGetter(src),
		b.NewPutter(src),
		func(fn func(Getter) error) error {
			return src.Snapshot(func(g Getter) error {
				return fn(b.NewGetter(g))
			})
		},
		func(fn func(Putter) error) error {
			return src.Batch(func(p Putter) error {
				return fn(b.NewPutter(p))
			})
		},
		func(rng Range, fn func(Pair) (bool, error)) error {
			{
				buf := bufPool.Get().(*buf)
				defer bufPool.Put(buf)
				buf.k = append(append(buf.k[:0], b...), rng.Start...)
				rng.Start = buf.k
			}

			if len(rng.Limit) == 0 {
				rng.Limit = util.BytesPrefix([]byte(b)).Limit
			} else {
				buf := bufPool.Get().(*buf)
				defer bufPool.Put(buf)
				buf.k = append(append(buf.k[:0], b...), rng.Limit...)
				rng.Limit = buf.k
			}

			var cur Pair
			pair := &struct {
				KeyFunc
				ValueFunc
			}{
				func() []byte { return cur.Key()[len(b):] },
				func() []byte { return cur.Value() },
			}

			return src.Iterate(rng, func(p Pair) (bool, error) {
				cur = p
				return fn(pair)
			})
		},
	}
}

type buf struct {
	k []byte
}

var bufPool = sync.Pool{
	New: func() interface{} {
		return &buf{}
	},
}
