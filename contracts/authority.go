package contracts

import (
	"encoding/binary"

	"github.com/vechain/thor/thor"
)

// Proposer data struct to communicate with `Authority` contract.
type Proposer struct {
	Address thor.Address
	Status  uint32
}

func (p *Proposer) encode() (encoded [32]byte) {
	copy(encoded[12:], p.Address[:])
	binary.BigEndian.PutUint32(encoded[8:], p.Status)
	return
}

func (p *Proposer) decode(encoded [32]byte) {
	copy(p.Address[:], encoded[12:])
	p.Status = binary.BigEndian.Uint32(encoded[8:])
}

type authority struct {
	contract
}

// PackInitialize pack input data of `Authority._initialize` function.
func (a *authority) PackInitialize(voting thor.Address) []byte {
	return a.mustPack("_initialize", voting)
}

// PackPreset pack input data of `Authority._preset` function.
func (a *authority) PackPreset(addr thor.Address, identity string) []byte {
	return a.mustPack("_preset", addr, identity)
}

// PackUpdate pack input data of `Authority._update` function.
func (a *authority) PackUpdate(proposers []Proposer) []byte {
	encoded := make([][32]byte, len(proposers))
	for i, p := range proposers {
		encoded[i] = p.encode()
	}
	return a.mustPack("_udpate", encoded)
}

// PackProposers pack input data of `Authority.proposers` function.
func (a *authority) PackProposers() []byte {
	return a.mustPack("proposers")
}

// UnpackProposers unpack return data of `Authority.proposers` function.
func (a *authority) UnpackProposers(output []byte) (proposers []Proposer) {
	var encoded [][32]byte
	a.mustUnpack(&encoded, "proposers", output)
	if len(encoded) > 0 {
		proposers = make([]Proposer, len(encoded))
		for i, e := range encoded {
			proposers[i].decode(e)
		}
	}
	return
}

// PackProposer pack input data of `Authority.proposer` function.
func (a *authority) PackProposer(addr thor.Address) []byte {
	return a.mustPack("proposer", addr)
}

// UnpackProposer unpack return data of `Authority.proposer` function.
func (a *authority) UnpackProposer(output []byte) (uint32, string) {
	var v struct {
		Status   uint32
		Identity string
	}
	a.mustUnpack(&v, "proposer", output)
	return v.Status, v.Identity
}

// Authority binder of `Authority` contract.
var Authority = authority{mustLoad(
	thor.BytesToAddress([]byte("poa")),
	"compiled/Authority.abi",
	"compiled/Authority.bin-runtime")}
