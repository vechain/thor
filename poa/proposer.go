package poa

import (
	"encoding/binary"

	"github.com/vechain/thor/thor"
)

// Proposer address with status.
type Proposer struct {
	Address thor.Address
	Status  uint32
}

// IsOnline returns if its status marked to online.
func (p *Proposer) IsOnline() bool {
	return (p.Status & uint32(1)) == 1
}

// SetOnline marks online or not.
func (p *Proposer) SetOnline(online bool) {
	if online {
		p.Status |= uint32(1)
	} else {
		p.Status &= uint32(0xfffffffe)
	}
}

// Encode encode to byte array.
func (p *Proposer) Encode() (encoded [32]byte) {
	copy(encoded[12:], p.Address[:])
	binary.BigEndian.PutUint32(encoded[8:], p.Status)
	return
}

// Decode decode from byte array.
func (p *Proposer) Decode(encoded [32]byte) {
	copy(p.Address[:], encoded[12:])
	p.Status = binary.BigEndian.Uint32(encoded[8:])
}
