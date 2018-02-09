package sslot

import (
	"encoding/binary"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/rlp"
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

// Array provides array access to 'slot'.
type Array struct {
	ss         *StorageSlot
	startIndex *big.Int
}

// NewArray create a new Array instance.
func NewArray(address thor.Address, slot uint32) *Array {
	ss := New(address, slot)
	return &Array{
		ss,
		new(big.Int).SetBytes(crypto.Keccak256(ss.position[:])),
	}
}

// Len returns length of array.
func (a *Array) Len(state *state.State) (l uint64) {
	a.ss.LoadStructed(state, (*stgUInt64)(&l))
	return
}

// SetLen reset length of array.
// If new length is smaller, elements that are out of range will be erased.
func (a *Array) SetLen(state *state.State, newLen uint64) {
	curLen := a.Len(state)
	for i := newLen; i < curLen; i++ {
		a.ForIndex(i).SaveStructed(state, nil)
	}
	a.ss.LoadStructed(state, stgUInt64(newLen))
}

// Append appends a new element, and returns new length.
func (a *Array) Append(state *state.State, elem interface{}) uint64 {
	l := a.Len(state)
	a.ForIndex(l).SaveStructed(state, elem)
	l++
	a.ss.SaveStructed(state, stgUInt64(l))
	return l
}

// ForIndex create a new StorageSlot for accessing element at given index.
func (a *Array) ForIndex(index uint64) *StorageSlot {
	x := new(big.Int).SetUint64(index)
	x.Add(x, a.startIndex)
	return &StorageSlot{
		a.ss.address,
		thor.BytesToHash(x.Bytes()),
	}
}

type stgUInt64 uint64

var _ state.StorageDecoder = (*stgUInt64)(nil)
var _ state.StorageEncoder = (*stgUInt64)(nil)

func (s *stgUInt64) Encode() ([]byte, error) {
	if *s == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(s)
}

func (s *stgUInt64) Decode(data []byte) error {
	if len(data) == 0 {
		*s = 0
		return nil
	}
	return rlp.DecodeBytes(data, s)
}
