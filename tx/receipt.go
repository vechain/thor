package tx

import (
	"github.com/vechain/thor/thor"
)

// Receipt represents the results of a transaction.
type Receipt struct {
	// gas used by this tx
	GasUsed uint64
	// the one who payed for gas
	GasPayer thor.Address
	// outputs of clauses in tx
	Outputs []*Output
}

// Output output of clause execution.
type Output struct {
	// logs produced by the clause
	Logs []*Log
}
