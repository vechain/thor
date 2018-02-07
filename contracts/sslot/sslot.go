package sslot

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

// StorageSlot similar concept as solidity's storage layout slot.
// The slot is bound to address and slot index.
// Note that it's NOT compiant with solidity storage layout.
type StorageSlot struct {
	address thor.Address
	slot    uint32

	dataKey  thor.Hash
	indexKey thor.Hash
}

// New create a slot instance.
func New(address thor.Address, slot uint32) *StorageSlot {
	var dataKey thor.Hash
	binary.BigEndian.PutUint32(dataKey[thor.HashLength-4:], slot)

	return &StorageSlot{
		address,
		slot,
		dataKey,
		thor.Hash(crypto.Keccak256Hash(dataKey[:])),
	}
}

// Get get value for given key.
// 'val' is to recevei decoded value, and it should implement
// state.StorageDecoder or rlp decodable.
func (ss *StorageSlot) Get(state *state.State, key thor.Hash, val interface{}) {
	state.GetStructedStorage(ss.address, key, val)
}

// Set set value for given key.
// 'val' should implement state.StorageEncoder or rlp encodable.
func (ss *StorageSlot) Set(state *state.State, key thor.Hash, val interface{}) {
	state.SetStructedStorage(ss.address, key, val)
}

// DataKey returns the key for accessing slot data.
func (ss *StorageSlot) DataKey() thor.Hash {
	return ss.dataKey
}

// IndexKey computes the key for accessing slot as an array.
func (ss *StorageSlot) IndexKey(index uint32) thor.Hash {
	var bytes [4]byte
	binary.BigEndian.PutUint32(bytes[:], index)
	ik := ss.indexKey
	var overflow uint
	for i := range ik {
		i0 := thor.HashLength - i - 1
		i1 := 4 - i - 1
		var sum uint
		if i1 >= 0 {
			sum = uint(ik[i0]) + uint(bytes[i1]) + overflow
		} else {
			sum = uint(ik[i0]) + overflow
		}
		if sum > 255 {
			overflow = 1
		} else {
			overflow = 0
		}
		ik[i0] = byte(sum % 256)
	}
	return ik
}

// MapKey computes the key for accessing slot as an map.
func (ss *StorageSlot) MapKey(key interface{}) (mk thor.Hash) {
	hw := sha3.NewKeccak256()
	err := rlp.Encode(hw, []interface{}{ss.dataKey, key})
	if err != nil {
		panic(err)
	}
	hw.Sum(mk[:0])
	return
}
