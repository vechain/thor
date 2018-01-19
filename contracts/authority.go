package contracts

import (
	"github.com/vechain/thor/schedule"
	"github.com/vechain/thor/thor"
)

type authority struct {
	contract
}

// PackInitialize pack input data of `Authority.sysInitialize` function.
func (a *authority) PackInitialize(voting thor.Address) []byte {
	return a.mustPack("sysInitialize", voting)
}

// PackPreset pack input data of `Authority.sysPreset` function.
func (a *authority) PackPreset(addr thor.Address, identity string) []byte {
	return a.mustPack("sysPreset", addr, identity)
}

// PackUpdate pack input data of `Authority.sysUpdate` function.
func (a *authority) PackUpdate(proposers []schedule.Proposer) []byte {
	encoded := make([][32]byte, len(proposers))
	for i, p := range proposers {
		encoded[i] = p.Encode()
	}
	return a.mustPack("sysUdpate", encoded)
}

// PackProposers pack input data of `Authority.proposers` function.
func (a *authority) PackProposers() []byte {
	return a.mustPack("proposers")
}

// UnpackProposers unpack return data of `Authority.proposers` function.
func (a *authority) UnpackProposers(output []byte) (proposers []schedule.Proposer) {
	var encoded [][32]byte
	a.mustUnpack(&encoded, "proposers", output)
	if len(encoded) > 0 {
		proposers = make([]schedule.Proposer, len(encoded))
		for i, e := range encoded {
			proposers[i].Decode(e)
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
