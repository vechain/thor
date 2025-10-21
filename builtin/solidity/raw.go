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
	context *Context
	pos     thor.Bytes32
}

// NewRaw creates a new Raw instance at the given storage position.
// Raw is a simple wrapper for a single storage slot of simple types(1 word gas cost).
func NewRaw[V comparable](context *Context, pos thor.Bytes32) *Raw[V] {
	return &Raw[V]{context: context, pos: pos}
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
	size, err := r.set(value)
	if err != nil {
		return err
	}
	r.context.UseGas(size * thor.SstoreSetGas)
	return nil
}

// Update update the value for the given key, charging reset value gas.
func (r *Raw[V]) Update(value V) error {
	size, err := r.set(value)
	if err != nil {
		return err
	}
	r.context.UseGas(size * thor.SstoreResetGas)
	return nil
}

func (r *Raw[V]) get() (V, error) {
	// prepare a zero-value container for decoding
	var value V

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

func (r *Raw[V]) set(value V) (uint64, error) {
	// do not RLP-encode nil values, instead set raw storage to nil
	var zero V
	if value == zero {
		r.context.state.SetRawStorage(r.context.address, r.pos, nil)
		return 0, nil // no gas charged for setting nil
	}

	// encode and store
	if err := r.context.state.EncodeStorage(r.context.address, r.pos, func() ([]byte, error) {
		// encode via rlp library's internal pooling
		buf, err := rlp.EncodeToBytes(value)
		if err != nil {
			return nil, err
		}
		return buf, nil
	}); err != nil {
		return 0, err
	}

	return 1, nil
}
