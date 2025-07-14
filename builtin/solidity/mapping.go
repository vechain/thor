// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"reflect"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/thor"
)

type Key interface {
	Bytes() []byte
}

// Mapping is a key/value storage abstraction for built-in contracts, similar to the mapping in Solidity.
// It DOES NOT (TBD) allow for direct access to values if declared in the same `pos` in the built-in contract.
type Mapping[K Key, V any] struct {
	context *Context
	basePos thor.Bytes32
}

func NewMapping[K Key, V any](context *Context, pos thor.Bytes32) *Mapping[K, V] {
	return &Mapping[K, V]{context: context, basePos: pos}
}

func (m *Mapping[K, V]) Get(key K) (value V, err error) {
	position := thor.Blake2b(key.Bytes(), m.basePos.Bytes())
	err = m.context.state.DecodeStorage(m.context.address, position, func(raw []byte) error {
		if reflect.ValueOf(value).Kind() == reflect.Ptr {
			value = reflect.New(reflect.TypeOf(value).Elem()).Interface().(V)
		}
		if len(raw) == 0 {
			return nil
		}
		slots := (uint64(len(raw)) + 31) / 32
		m.context.UseGas(slots * thor.SloadGas)
		return rlp.DecodeBytes(raw, &value)
	})
	return
}

func (m *Mapping[K, V]) Set(key K, value V, newValue bool) error {
	position := thor.Blake2b(key.Bytes(), m.basePos.Bytes())
	return m.context.state.EncodeStorage(m.context.address, position, func() ([]byte, error) {
		val, err := rlp.EncodeToBytes(value)
		if err != nil {
			return nil, err
		}
		slots := (uint64(len(val)) + 31) / 32
		if newValue {
			m.context.UseGas(slots * thor.SstoreSetGas)
		} else {
			m.context.UseGas(slots * thor.SstoreResetGas)
		}
		return rlp.EncodeToBytes(value)
	})
}
