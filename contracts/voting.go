package contracts

import (
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// Voting binder of `Voting` contract.
var Voting = func() voting {
	addr := thor.BytesToAddress([]byte("vot"))
	return voting{
		addr,
		mustLoad("compiled/Voting.abi", "compiled/Voting.bin-runtime"),
		tx.NewClause(&addr),
	}
}()

type voting struct {
	Address thor.Address
	contract
	clause *tx.Clause
}
