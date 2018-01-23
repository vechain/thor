package contracts

import "github.com/vechain/thor/thor"

type voting struct {
	contract
}

// Voting binder of `Voting` contract.
var Voting = voting{mustLoad(
	thor.BytesToAddress([]byte("vot")),
	"compiled/Voting.abi",
	"compiled/Voting.bin-runtime")}
