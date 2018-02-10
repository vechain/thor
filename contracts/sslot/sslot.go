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

// LoadStructed load structed value.
// 'val' is to recevei decoded value, and it should implement
// state.StorageDecoder or rlp decodable.
func (ss *StorageSlot) LoadStructed(state *state.State, val interface{}) {
	state.GetStructedStorage(ss.address, ss.position, val)
}

// SaveStructed save structed value.
// 'val' should implement state.StorageEncoder or rlp encodable.
// If 'val is nil, corresponded storage will be cleared.
func (ss *StorageSlot) SaveStructed(state *state.State, val interface{}) {
	state.SetStructedStorage(ss.address, ss.position, val)
}

// Load load value as machine word.
func (ss *StorageSlot) Load(state *state.State) thor.Hash {
	return state.GetStorage(ss.address, ss.position)
}

// Save save value as machine word.
func (ss *StorageSlot) Save(state *state.State, val thor.Hash) {
	state.SetStorage(ss.address, ss.position, val)
}
