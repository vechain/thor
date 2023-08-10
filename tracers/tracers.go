// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package tracers is a manager for transaction tracing engines.
package tracers

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm"
)

// Context contains some contextual infos for a transaction execution that is not
// available from within the EVM object.
type Context struct {
	BlockID     thor.Bytes32 // Hash of the block the tx is contained within (zero if dangling tx or call)
	BlockTime   uint64       // Timestamp of the block the tx is contained within
	TxIndex     int          // Index of the transaction within a block (zero if dangling tx or call)
	TxID        thor.Bytes32 // ID of the transaction being traced (zero if dangling call)
	ClauseIndex int          // Index of the clause within a transaction (zero if dangling call)
	State       *state.State
}

// Tracer interface extends vm.EVMLogger and additionally
// allows collecting the tracing result.
type Tracer interface {
	vm.Logger
	SetContext(*Context)
	GetResult() (json.RawMessage, error)
	// Stop terminates execution of the tracer at the first opportune moment.
	Stop(err error)
}

type ctorFn func(json.RawMessage) (Tracer, error)
type jsCtorFn func(string, json.RawMessage) (Tracer, error)

type elem struct {
	ctor ctorFn
	isJS bool
}

// DefaultDirectory is the collection of tracers bundled by default.
var DefaultDirectory = directory{elems: make(map[string]elem)}

// directory provides functionality to lookup a tracer by name
// and a function to instantiate it. It falls back to a JS code evaluator
// if no tracer of the given name exists.
type directory struct {
	elems  map[string]elem
	jsEval jsCtorFn
}

// Register registers a method as a lookup for tracers, meaning that
// users can invoke a named tracer through that lookup.
func (d *directory) Register(name string, f ctorFn, isJS bool) {
	d.elems[name] = elem{ctor: f, isJS: isJS}
}

// RegisterJSEval registers a tracer that is able to parse
// dynamic user-provided JS code.
func (d *directory) RegisterJSEval(f jsCtorFn) {
	d.jsEval = f
}

// New returns a new instance of a tracer, by iterating through the
// registered lookups. Name is either name of an existing tracer
// or an arbitrary JS code.
func (d *directory) New(name string, cfg json.RawMessage, allowCustom bool) (Tracer, error) {
	if elem, ok := d.elems[name]; ok {
		return elem.ctor(cfg)
	}
	// backward compatible, allow users emit "Tracer" suffix
	if elem, ok := d.elems[name+"Tracer"]; ok {
		return elem.ctor(cfg)
	}

	if allowCustom {
		// Assume JS code
		tracer, err := d.jsEval(name, cfg)
		if err != nil {
			return nil, errors.Wrap(err, "create custom tracer")
		}
		return tracer, nil
	} else {
		return nil, errors.New("unsupported tracer")
	}
}

// IsJS will return true if the given tracer will evaluate
// JS code. Because code evaluation has high overhead, this
// info will be used in determining fast and slow code paths.
func (d *directory) IsJS(name string) bool {
	if elem, ok := d.elems[name]; ok {
		return elem.isJS
	}
	// JS eval will execute JS code
	return true
}

const (
	memoryPadLimit = 1024 * 1024
)

// GetMemoryCopyPadded returns offset + size as a new slice.
// It zero-pads the slice if it extends beyond memory bounds.
func GetMemoryCopyPadded(m *vm.Memory, offset, size int64) ([]byte, error) {
	if offset < 0 || size < 0 {
		return nil, errors.New("offset or size must not be negative")
	}
	if int(offset+size) < m.Len() { // slice fully inside memory
		return m.GetCopy(offset, size), nil
	}
	paddingNeeded := int(offset+size) - m.Len()
	if paddingNeeded > memoryPadLimit {
		return nil, fmt.Errorf("reached limit for padding memory slice: %d", paddingNeeded)
	}
	cpy := make([]byte, size)
	if overlap := int64(m.Len()) - offset; overlap > 0 {
		copy(cpy, m.GetPtr(offset, overlap))
	}
	return cpy, nil
}
