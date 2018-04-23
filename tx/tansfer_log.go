package tx

import (
	"math/big"

	"github.com/vechain/thor/thor"
)

// TransferLog token transfer log.
type TransferLog struct {
	Sender    thor.Address
	Recipient thor.Address
	Amount    *big.Int
}
