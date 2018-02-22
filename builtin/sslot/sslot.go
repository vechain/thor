package sslot

import (
	"encoding/binary"

	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

// StorageSlot entry to access account storage.
// Note: it's not compliant with Solidity storage layout.
type StorageSlot struct {
	address  thor.Address
	position thor.Hash
}

// New create a slot instance.
func New(address thor.Address, slot uint32) *StorageSlot {
	var position thor.Hash
	binary.BigEndian.PutUint32(position[thor.HashLength-4:], slot)

	return &StorageSlot{
		address,
		position,
	}
}

// Load load value.
// 'val' is to recevei decoded value. See state.State.GetStructedStorage.
func (ss *StorageSlot) Load(state *state.State, val interface{}) {
	state.GetStructedStorage(ss.address, ss.position, val)
}

// Save save value. See state.State.SetStructedStorage.
func (ss *StorageSlot) Save(state *state.State, val interface{}) {
	state.SetStructedStorage(ss.address, ss.position, val)
}
