// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package kv

// Getter defines methods to read kv.
type Getter interface {
	Get(key []byte) ([]byte, error)
	Has(key []byte) (bool, error)
	IsNotFound(err error) bool
}

// Putter defines methods to write kv.
type Putter interface {
	Put(key, val []byte) error
	Delete(key []byte) error
}

// Snapshot is the store's snapshot.
type Snapshot interface {
	Getter
	Release()
}

// Bulk is the bulk putter.
type Bulk interface {
	Putter
	EnableAutoFlush() // if set, the bulk will be non-atomic
	Write() error
}

// Iterator iterates over kv pairs.
type Iterator interface {
	First() bool
	Last() bool
	Next() bool
	Prev() bool
	Key() []byte
	Value() []byte
	Release()
	Error() error
}

// Range is the key range.
type Range struct {
	Start []byte // start of key range (included)
	Limit []byte // limit of key range (excluded)
}

// Store defines the full functional kv store.
type Store interface {
	Getter
	Putter

	Snapshot() Snapshot
	Bulk() Bulk
	Iterate(r Range) Iterator
}
