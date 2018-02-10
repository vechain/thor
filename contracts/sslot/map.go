package sslot

import (
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
)

// Map provides map access to at 'slot'.
type Map StorageSlot

// NewMap create a Map instance.
func NewMap(address thor.Address, slot uint32) *Map {
	return (*Map)(New(address, slot))
}

func (m *Map) transformKey(key interface{}) (keyPos thor.Hash) {
	hw := sha3.NewKeccak256()
	err := rlp.Encode(hw, []interface{}{m.position, key})
	if err != nil {
		panic(err)
	}
	hw.Sum(keyPos[:0])
	return
}

// ForKey create a new StorageSlot for accessing value for given key.
func (m *Map) ForKey(key interface{}) *StorageSlot {
	keyPos := m.transformKey(key)
	return &StorageSlot{
		m.address,
		keyPos,
	}
}
