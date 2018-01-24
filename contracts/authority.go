package contracts

import (
	"github.com/vechain/thor/schedule"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// Authority binder of `Authority` contract.
var Authority = func() authority {
	addr := thor.BytesToAddress([]byte("poa"))
	return authority{
		addr,
		mustLoad("compiled/Authority.abi", "compiled/Authority.bin-runtime"),
		tx.NewClause(&addr),
	}
}()

type authority struct {
	Address thor.Address
	contract
	clause *tx.Clause
}

// PackInitialize pack input data of `Authority.sysInitialize` function.
func (a *authority) PackInitialize(voting thor.Address) *tx.Clause {
	return a.clause.WithData(a.mustPack("sysInitialize", voting))
}

// PackPreset pack input data of `Authority.sysPreset` function.
func (a *authority) PackPreset(addr thor.Address, identity string) *tx.Clause {
	return a.clause.WithData(a.mustPack("sysPreset", addr, identity))
}

// PackUpdate pack input data of `Authority.sysUpdate` function.
func (a *authority) PackUpdate(proposers []schedule.Proposer) *tx.Clause {
	encoded := make([][32]byte, len(proposers))
	for i, p := range proposers {
		encoded[i] = p.Encode()
	}
	return a.clause.WithData(a.mustPack("sysUdpate", encoded))
}

// PackProposers pack input data of `Authority.proposers` function.
func (a *authority) PackProposers() *tx.Clause {
	return a.clause.WithData(a.mustPack("proposers"))
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
func (a *authority) PackProposer(addr thor.Address) *tx.Clause {
	return a.clause.WithData(a.mustPack("proposer", addr))
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
