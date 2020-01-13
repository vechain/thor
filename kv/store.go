// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package kv

// Getter defines methods to read kv.
type Getter interface {
	Get(key []byte) ([]byte, error)
	Has(key []byte) (bool, error)
}

// Putter defines methods to write kv.
type Putter interface {
	Put(key, val []byte) error
	Delete(key []byte) error
}

// PutFlusher defines putter with Flush method.
type PutFlusher interface {
	Putter
	Flush() error
}

// Pair defines key-value pair.
type Pair interface {
	Key() []byte
	Value() []byte
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

	Snapshot(fn func(Getter) error) error
	Batch(fn func(PutFlusher) error) error
	Iterate(r Range, fn func(Pair) bool) error
	IsNotFound(err error) bool
}
