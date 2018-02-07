package contracts

import (
	"github.com/vechain/thor/thor"
)

// Voting binder of `Voting` contract.
var Voting = &voting{thor.BytesToAddress([]byte("vot"))}

//"compiled/Voting.abi"

type voting struct {
	Address thor.Address
}

func (v *voting) RuntimeBytecodes() []byte {
	return mustLoadHexData("compiled/Voting.bin-runtime")
}
