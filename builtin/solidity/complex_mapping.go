// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

import (
	"math/big"
	"reflect"

	"github.com/vechain/thor/v2/thor"
)

type ComplexValue [T any]interface {
	DecodeSlots(slots []thor.Bytes32) error
	EncodeSlots() []thor.Bytes32
	UsedSlots() int
}

type ComplexMapping[K Key, V ComplexValue[any]] struct {
	context *Context
	basePos thor.Bytes32
}

// NewComplexMapping creates a new persistent mapping at the given storage position.
func NewComplexMapping[K Key, V ComplexValue[any]](context *Context, pos thor.Bytes32) *ComplexMapping[K, V] {
	return &ComplexMapping[K, V]{context: context, basePos: pos}
}

func (m *ComplexMapping[K, V]) Get(key K) (V, error) {
	// compute base slot = keccak256(key + basePos)
	keyBytes32 := thor.BytesToBytes32(key.Bytes())
	base := thor.Keccak256(keyBytes32.Bytes(), m.basePos.Bytes()).Bytes()

	var output V

	// If V is a pointer type, we need to initialize it
	if reflect.TypeOf(output).Kind() == reflect.Ptr {
		output = reflect.New(reflect.TypeOf(output).Elem()).Interface().(V)
	}

	slotsUsed := output.UsedSlots()
	storage := make([]thor.Bytes32, slotsUsed)

	for i := 0; i < slotsUsed; i++ {
		slot := IncrementSlot(base, i)
		data, err := m.context.state.GetStorage(m.context.address, slot)
		if err != nil {
			return output, err
		}
		storage[i] = data
	}

	if err := output.DecodeSlots(storage); err != nil {
		return output, err
	}

	return output, nil
}

func (m *ComplexMapping[K, V]) Set(key K, value V) {
	keyBytes32 := thor.BytesToBytes32(key.Bytes())
	base := thor.Keccak256(keyBytes32.Bytes(), m.basePos.Bytes()).Bytes()
	slots := value.EncodeSlots()

	for i, slotValue := range slots {
		slot := IncrementSlot(base, i)
		m.context.state.SetStorage(m.context.address, slot, slotValue)
	}
}

// IncrementSlot adds i to the base slot (32-byte array interpreted as big.Int)
// and returns the resulting slot as a 32-byte array.
func IncrementSlot(base []byte, i int) thor.Bytes32 {
	baseInt := new(big.Int).SetBytes(base)            // Convert base slot bytes to big.Int
	slotInt := new(big.Int).Add(baseInt, big.NewInt(int64(i))) // Add offset i
	var slotBytes [32]byte
	// FillBytes fills slotBytes from the right with big-endian representation
	slotInt.FillBytes(slotBytes[:])
	return slotBytes
}

func NumToSlot(num uint8) thor.Bytes32 {
	var slot thor.Bytes32
	slot[31] = num // Store the number in the last byte
	return slot
}
