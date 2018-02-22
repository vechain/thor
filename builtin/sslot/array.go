package sslot

import (
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
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
func (a *Array) Len(state *state.State) (length uint64) {
	a.ss.Load(state, &length)
	return
}

// SetLen reset length of array.
// If new length is smaller, elements that are out of range will be erased.
func (a *Array) SetLen(state *state.State, newLen uint64) {
	curLen := a.Len(state)
	for i := newLen; i < curLen; i++ {
		a.ForIndex(i).Save(state, nil)
	}
	a.ss.Save(state, newLen)
}

// Append appends a new element, and returns new length.
func (a *Array) Append(state *state.State, elem interface{}) uint64 {
	l := a.Len(state)
	a.ForIndex(l).Save(state, elem)
	l++
	a.ss.Save(state, l)
	return l
}

// ForIndex create a new StorageSlot for accessing element at given index.
// TODO: check the case index is out of bound
func (a *Array) ForIndex(index uint64) *StorageSlot {
	x := new(big.Int).SetUint64(index)
	x.Add(x, a.startIndex)
	return &StorageSlot{
		a.ss.address,
		thor.BytesToHash(x.Bytes()),
	}
}
