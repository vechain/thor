package schedule

import (
	"encoding/binary"

	"github.com/vechain/thor/thor"
)

// Proposer address with status.
type Proposer struct {
	Address thor.Address
	Status  uint32
}

// IsAbsent returns if its status marked to absent.
func (p *Proposer) IsAbsent() bool {
	return (p.Status & uint32(1)) != 0
}

// SetAbsent marks absent or not.
func (p *Proposer) SetAbsent(b bool) {
	if b {
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
