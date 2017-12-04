package tx

import (
	"math/big"
)

// Receipt represents the results of a transaction.
type Receipt struct {
	// status of tx execution
	Status uint
	// which clause caused tx failure
	BadClauseIndex uint
	// gas used by this tx
	GasUsed *big.Int
	// logs produced
	Logs []*Log
}
