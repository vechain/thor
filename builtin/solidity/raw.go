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

func NewRaw[V comparable](context *Context, pos thor.Bytes32) *Raw[V] {
	return &Raw[V]{context: context, pos: pos}
}

func (r *Raw[V]) Get() (V, error) {
	// prepare a zero-value container for decoding
	var value V

	// attempt to decode storage into value
	err := r.context.state.DecodeStorage(r.context.address, r.pos, func(raw []byte) error {
		r.context.UseGas(thor.SloadGas)

		if len(raw) == 0 {
			return nil
		}

		// decode RLP in-place
		return rlp.DecodeBytes(raw, &value)
	})

	return value, err
}

func (r *Raw[V]) Set(value V) error {
	// do not RLP-encode nil values, instead set raw storage to nil
	var zero V
	if value == zero {
		r.context.state.SetRawStorage(r.context.address, r.pos, nil)
		return nil
	}

	// encode and store
	return r.context.state.EncodeStorage(r.context.address, r.pos, func() ([]byte, error) {
		// encode via rlp library's internal pooling
		buf, err := rlp.EncodeToBytes(value)
		if err != nil {
			return nil, err
		}

		r.context.UseGas(thor.SstoreResetGas)
		return buf, nil
	})
}
