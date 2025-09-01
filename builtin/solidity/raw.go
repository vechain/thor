// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/thor"
)

type Raw[V comparable] struct {
	context    *Context
	pos        thor.Bytes32
	directSlot bool // when true, store directly in the 32-byte slot (no RLP)
}

// NewRaw creates a new Raw instance at the given storage position.
// Raw is a simple wrapper for a single storage slot of simple types(1 word gas cost).
func NewRaw[V comparable](context *Context, pos thor.Bytes32) *Raw[V] {
	return &Raw[V]{context: context, pos: pos}
}

// NewAddress returns a Raw[thor.Address] configured to use slot storage (no RLP).
func NewAddress(ctx *Context, pos thor.Bytes32) *Raw[thor.Address] {
	r := NewRaw[thor.Address](ctx, pos)
	r.directSlot = true
	return r
}

// Get retrieves the value for the given key, charging SloadGas.
func (r *Raw[V]) Get() (V, error) {
	r.context.UseGas(thor.SloadGas)
	return r.get()
}

// Upsert update or insert the value for the given key, charging different gas based on Insert or Update.
func (r *Raw[V]) Upsert(value V) error {
	// prepare a zero-value container for comparison
	var zero V

	prev, err := r.get()
	if err != nil {
		return err
	}

	if prev == zero {
		return r.Insert(value)
	}
	return r.Update(value)
}

// Insert insert a new value for the given key, charging new value gas.
func (r *Raw[V]) Insert(value V) error {
	r.context.UseGas(thor.SstoreSetGas)
	return r.set(value)
}

// Update update the value for the given key, charging reset value gas.
func (r *Raw[V]) Update(value V) error {
	r.context.UseGas(thor.SstoreResetGas)
	return r.set(value)
}

func (r *Raw[V]) get() (V, error) {
	// prepare a zero-value container for decoding
	var value V

	if r.directSlot { // directSlot is only set by NewAddress
		word, err := r.context.state.GetStorage(r.context.address, r.pos)
		if err != nil {
			return value, err
		}

		return any(thor.BytesToAddress(word.Bytes())).(V), nil
	}

	// attempt to decode storage into value
	err := r.context.state.DecodeStorage(r.context.address, r.pos, func(raw []byte) error {
		if len(raw) == 0 {
			return nil
		}

		// decode RLP in-place
		return rlp.DecodeBytes(raw, &value)
	})

	return value, err
}

func (r *Raw[V]) set(value V) error {
	// do not RLP-encode nil values, instead set raw storage to nil
	var zero V
	if value == zero {
		r.context.state.SetRawStorage(r.context.address, r.pos, nil)
		return nil
	}

	if r.directSlot { // directSlot is only set by NewAddress
		a := any(value).(thor.Address)
		r.context.state.SetStorage(r.context.address, r.pos, thor.BytesToBytes32(a.Bytes()))
		return nil
	}

	// encode and store
	return r.context.state.EncodeStorage(r.context.address, r.pos, func() ([]byte, error) {
		// encode via rlp library's internal pooling
		buf, err := rlp.EncodeToBytes(value)
		if err != nil {
			return nil, err
		}
		return buf, nil
	})
}
