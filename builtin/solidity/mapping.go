// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"reflect"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

type Key interface {
	Bytes() []byte
}

// Mapping is a key/value storage abstraction for built-in contracts, similar to the mapping in Solidity.
// It DOES NOT (TBD) allow for direct access to values if declared in the same `pos` in the built-in contract.
type Mapping[K Key, V any] struct {
	addr    thor.Address
	basePos thor.Bytes32
	state   *state.State
}

func NewMapping[K Key, V any](addr thor.Address, state *state.State, pos thor.Bytes32) *Mapping[K, V] {
	return &Mapping[K, V]{addr: addr, state: state, basePos: pos}
}

func (m *Mapping[K, V]) Get(key K) (value V, err error) {
	position := thor.Blake2b(key.Bytes(), m.basePos.Bytes())
	err = m.state.DecodeStorage(m.addr, position, func(raw []byte) error {
		if reflect.ValueOf(value).Kind() == reflect.Ptr {
			value = reflect.New(reflect.TypeOf(value).Elem()).Interface().(V)
		}

		if len(raw) == 0 {
			return nil
		}
		return rlp.DecodeBytes(raw, &value)
	})
	return
}

func (m *Mapping[K, V]) Set(key K, value V) error {
	position := thor.Blake2b(key.Bytes(), m.basePos.Bytes())
	return m.state.EncodeStorage(m.addr, position, func() ([]byte, error) {
		return rlp.EncodeToBytes(value)
	})
}
