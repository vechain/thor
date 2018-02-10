package sslot

import (
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

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
