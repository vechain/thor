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
		BulkFunc
		IterateFunc
	}{
		b.NewGetter(src),
		b.NewPutter(src),
		func() Snapshot {
			snapshot := src.Snapshot()
			return &struct {
				Getter
				ReleaseFunc
			}{
				b.NewGetter(snapshot),
				snapshot.Release,
			}
		},
		func() Bulk {
			bulk := src.Bulk()
			return &struct {
				Putter
				EnableAutoFlushFunc
				WriteFunc
			}{
				b.NewPutter(bulk),
				bulk.EnableAutoFlush,
				bulk.Write,
			}
		},
		func(r Range) Iterator {
			{
				buf := bufPool.Get().(*buf)
				defer bufPool.Put(buf)
				buf.k = append(append(buf.k[:0], b...), r.Start...)
				r.Start = buf.k
			}

			if len(r.Limit) == 0 {
				r.Limit = util.BytesPrefix([]byte(b)).Limit
			} else {
				buf := bufPool.Get().(*buf)
				defer bufPool.Put(buf)
				buf.k = append(append(buf.k[:0], b...), r.Limit...)
				r.Limit = buf.k
			}
			iter := src.Iterate(r)
			return &struct {
				FirstFunc
				LastFunc
				NextFunc
				PrevFunc
				KeyFunc
				ValueFunc
				ReleaseFunc
				ErrorFunc
			}{
				iter.First,
				iter.Last,
				iter.Next,
				iter.Prev,
				// strip the bucket
				func() []byte { return iter.Key()[len(b):] },
				iter.Value,
				iter.Release,
				iter.Error,
			}
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
