// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package kv

// defines individual functions.

type (
	GetFunc        func(key []byte) ([]byte, error)
	HasFunc        func(key []byte) (bool, error)
	PutFunc        func(key, val []byte) error
	DeleteFunc     func(key []byte) error
	FlushFunc      func() error
	SnapshotFunc   func(fn func(Getter) error) error
	BatchFunc      func(fn func(PutFlusher) error) error
	IterateFunc    func(rgn Range, fn func(Pair) bool) error
	IsNotFoundFunc func(err error) bool
	KeyFunc        func() []byte
	ValueFunc      func() []byte
)

func (f GetFunc) Get(key []byte) ([]byte, error)                  { return f(key) }
func (f HasFunc) Has(key []byte) (bool, error)                    { return f(key) }
func (f PutFunc) Put(key, val []byte) error                       { return f(key, val) }
func (f DeleteFunc) Delete(key []byte) error                      { return f(key) }
func (f FlushFunc) Flush() error                                  { return f() }
func (f SnapshotFunc) Snapshot(fn func(Getter) error) error       { return f(fn) }
func (f BatchFunc) Batch(fn func(PutFlusher) error) error         { return f(fn) }
func (f IterateFunc) Iterate(rng Range, fn func(Pair) bool) error { return f(rng, fn) }
func (f IsNotFoundFunc) IsNotFound(err error) bool                { return f(err) }
func (f KeyFunc) Key() []byte                                     { return f() }
func (f ValueFunc) Value() []byte                                 { return f() }
