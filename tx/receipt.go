package tx

import (
	"github.com/vechain/thor/bn"
)

// Receipt represents the results of a transaction.
type Receipt struct {
	// gas used by this tx
	GasUsed bn.Int
	// outputs of clauses in tx
	Outputs []*Output
}

// Output output of clause execution.
type Output struct {
	// returned data of the clause that invokes a method of a contract
	ReturnData []byte
	// logs produced by the clause
	Logs []*Log
}
