// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"reflect"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

type Key interface {
	Bytes() []byte
}

// Mapping is a key/value storage abstraction for built-in contracts, similar to the mapping in Solidity.
// It DOES NOT (TBD) allow for direct access to values if declared in the same `pos` in the built-in contract.
type Mapping[K Key, V any] struct {
	addr      thor.Address
	basePos   thor.Bytes32
	state     *state.State
	charger   *gascharger.Charger
	retrieved map[Key][]byte
}

func NewMapping[K Key, V any](
	root *Root,
	pos thor.Bytes32,
) *Mapping[K, V] {
	return &Mapping[K, V]{
		addr:      root.address,
		state:     root.state,
		basePos:   pos,
		charger:   root.charger,
		retrieved: make(map[Key][]byte),
	}
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
		retrievedSlots := (len(raw) + 31) / 32
		if m.charger != nil {
			m.charger.Charge(thor.SloadGas * uint64(retrievedSlots))
		}
		m.retrieved[key] = raw
		return rlp.DecodeBytes(raw, &value)
	})
	return
}

func (m *Mapping[K, V]) Set(key K, value V) error {
	position := thor.Blake2b(key.Bytes(), m.basePos.Bytes())
	return m.state.EncodeStorage(m.addr, position, func() ([]byte, error) {
		newValue, err := rlp.EncodeToBytes(value)
		if err != nil {
			return nil, err
		}

		if m.charger != nil {
			if prevValue, exists := m.retrieved[key]; !exists {
				// New entry: charge for every slot used
				slotsUsed := (uint64(len(newValue)) + 31) / 32 // Round up to next slot boundary
				gas := thor.SstoreSetGas * slotsUsed
				m.charger.Charge(gas)
			} else {
				// Existing entry: compare slot by slot to determine gas cost
				m.chargeForSlotChanges(prevValue, newValue)
			}
		}

		return newValue, nil
	})
}

// chargeForSlotChanges compares old and new values slot by slot and charges gas accordingly
func (m *Mapping[K, V]) chargeForSlotChanges(oldValue, newValue []byte) {
	// Pad both values to slot boundaries (32 bytes each)
	oldSlots := padToSlots(oldValue)
	newSlots := padToSlots(newValue)

	// Determine the maximum number of slots to compare
	maxSlots := len(oldSlots)
	if len(newSlots) > maxSlots {
		maxSlots = len(newSlots)
	}

	for i := 0; i < maxSlots; i++ {
		var oldSlot, newSlot [32]byte

		// Get old slot value (zero if beyond old value length)
		if i < len(oldSlots) {
			oldSlot = oldSlots[i]
		}

		// Get new slot value (zero if beyond new value length)
		if i < len(newSlots) {
			newSlot = newSlots[i]
		}

		// Compare slots and charge gas based on the change
		if oldSlot != newSlot {
			oldSlotEmpty := isEmptySlot(oldSlot)
			newSlotEmpty := isEmptySlot(newSlot)

			if oldSlotEmpty && !newSlotEmpty {
				// Zero to non-zero: charge set gas
				m.charger.Charge(thor.SstoreSetGas)
			} else {
				// Non-zero to non-zero or non-zero to zero: charge reset gas
				m.charger.Charge(thor.SstoreResetGas)
			}
		}
	}
}

// padToSlots pads data to 32-byte slot boundaries
func padToSlots(data []byte) [][32]byte {
	if len(data) == 0 {
		return nil
	}

	numSlots := (len(data) + 31) / 32
	slots := make([][32]byte, numSlots)

	for i := 0; i < numSlots; i++ {
		start := i * 32
		end := start + 32
		if end > len(data) {
			end = len(data)
		}
		copy(slots[i][:], data[start:end])
	}

	return slots
}

// isEmptySlot checks if a slot contains all zeros
func isEmptySlot(slot [32]byte) bool {
	for _, b := range slot {
		if b != 0 {
			return false
		}
	}
	return true
}
