// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package kv

// defines individual functions.

type (
	GetFunc             func(key []byte) ([]byte, error)
	HasFunc             func(key []byte) (bool, error)
	IsNotFoundFunc      func(err error) bool
	PutFunc             func(key, val []byte) error
	DeleteFunc          func(key []byte) error
	SnapshotFunc        func() Snapshot
	BulkFunc            func() Bulk
	IterateFunc         func(r Range) Iterator
	EnableAutoFlushFunc func()
	WriteFunc           func() error
	FirstFunc           func() bool
	LastFunc            func() bool
	NextFunc            func() bool
	PrevFunc            func() bool
	KeyFunc             func() []byte
	ValueFunc           func() []byte
	ReleaseFunc         func()
	ErrorFunc           func() error
)

func (f GetFunc) Get(key []byte) ([]byte, error)   { return f(key) }
func (f HasFunc) Has(key []byte) (bool, error)     { return f(key) }
func (f IsNotFoundFunc) IsNotFound(err error) bool { return f(err) }
func (f PutFunc) Put(key, val []byte) error        { return f(key, val) }
func (f DeleteFunc) Delete(key []byte) error       { return f(key) }
func (f SnapshotFunc) Snapshot() Snapshot          { return f() }
func (f BulkFunc) Bulk() Bulk                      { return f() }
func (f IterateFunc) Iterate(r Range) Iterator     { return f(r) }
func (f EnableAutoFlushFunc) EnableAutoFlush()     { f() }
func (f WriteFunc) Write() error                   { return f() }
func (f FirstFunc) First() bool                    { return f() }
func (f LastFunc) Last() bool                      { return f() }
func (f NextFunc) Next() bool                      { return f() }
func (f PrevFunc) Prev() bool                      { return f() }
func (f KeyFunc) Key() []byte                      { return f() }
func (f ValueFunc) Value() []byte                  { return f() }
func (f ReleaseFunc) Release()                     { f() }
func (f ErrorFunc) Error() error                   { return f() }
