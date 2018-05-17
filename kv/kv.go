// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package kv

// Getter wraps methods for getting kvs.
type Getter interface {
	// Get value for given key.
	// An error returned if key not found. It can be checked via IsNotFound.
	Get(key []byte) (value []byte, err error)
	Has(key []byte) (bool, error)
	IsNotFound(error) bool

	NewIterator(r Range) Iterator
}

// Putter wraps methods for putting kvs.
type Putter interface {
	Put(key, value []byte) error
	Delete(key []byte) error

	NewBatch() Batch
}

// GetPutter wraps methods for getting/putting kvs.
type GetPutter interface {
	Getter
	Putter
}

// GetPutCloser with close method.
type GetPutCloser interface {
	GetPutter
	Close() error
}

// Batch defines batch of putting ops.
type Batch interface {
	Putter

	Len() int
	Write() error
}

// Iterator to iterates kvs.
type Iterator interface {
	Next() bool
	Release()
	Error() error

	Key() []byte
	Value() []byte
}
