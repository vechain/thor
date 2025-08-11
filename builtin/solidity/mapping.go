// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/thor"
)

type Key interface {
	Bytes() []byte
}

// Mapping is a key/value storage abstraction for built-in contracts, similar to the mapping in Solidity.
// It DOES NOT (TBD) allow for direct access to values if declared in the same `pos` in the built-in contract.
//
// Mapping behavior:
//
//   - Setting the default type value means removing the value from storage
//
//   - If Mapping[thor.Bytes32, *thor.Address]:
//     ex: SET (k:0x123, v:nil) -> stores nil (default empty value)
//
//   - If Mapping[thor.Bytes32, thor.Address]:
//     ex: SET (k:0x123, v:thor.Address{}) -> stores nil (default empty value)
//
//   - Getting a nil storage value will always return default value of V
//
//   - If Mapping[thor.Bytes32, *thor.Address]:
//     ex: SET (k:0x123, v:nil) GET (k:0x123)-> returns nil
//
//   - If Mapping[thor.Bytes32, thor.Address]:
//     ex: SET (k:0x123, v:thor.Address{}) GET (k:0x123)-> returns thor.Address{} (default empty value)
//
// - Getting a non-existing storage will always return the default value of the type defined in the V comparable mapping
type Mapping[K Key, V comparable] struct {
	context *Context
	basePos thor.Bytes32
}

// NewMapping creates a new persistent mapping at the given storage position.
func NewMapping[K Key, V comparable](context *Context, pos thor.Bytes32) *Mapping[K, V] {
	return &Mapping[K, V]{context: context, basePos: pos}
}

// Get retrieves the value for the given key, charging SloadGas per 32-byte word.
// If no value is present, returns the zero-value of V.
func (m *Mapping[K, V]) Get(key K) (V, error) {
	// compute the storage slot from key + base position
	position := thor.Blake2b(key.Bytes(), m.basePos.Bytes())

	// prepare a zero-value container for decoding
	var value V
	// attempt to decode storage into value
	err := m.context.state.DecodeStorage(
		m.context.address, position, func(raw []byte) error {
			if len(raw) == 0 {
				// use at least one SLOAD
				m.context.UseGas(thor.SloadGas)
				// no data, leave value as zero
				return nil
			}

			// charge gas per 32-byte word
			slots := (uint64(len(raw)) + 31) / 32
			m.context.UseGas(slots * thor.SloadGas)

			// decode RLP in-place
			return rlp.DecodeBytes(raw, &value)
		})
	if err != nil {
		return value, err
	}

	return value, nil
}

// Set stores the given value at key, charging Sstore gas per 32-byte word.
// Passing the zero-value of V clears the storage slot.
func (m *Mapping[K, V]) Set(key K, value V, newValue bool) error {
	position := thor.Blake2b(key.Bytes(), m.basePos.Bytes())

	// do not RLP-encode zero values, instead set raw storage to nil as remove operation
	var zero V
	if value == zero {
		m.context.state.SetRawStorage(m.context.address, position, nil)
		return nil
	}

	// encode and store
	return m.context.state.EncodeStorage(m.context.address, position, func() ([]byte, error) {
		// encode via rlp library's internal pooling
		buf, err := rlp.EncodeToBytes(value)
		if err != nil {
			return nil, err
		}

		// charge gas per 32-byte word
		slots := (uint64(len(buf)) + 31) / 32
		if newValue {
			m.context.UseGas(slots * thor.SstoreSetGas)
		} else {
			m.context.UseGas(slots * thor.SstoreResetGas)
		}
		return buf, nil
	})
}
