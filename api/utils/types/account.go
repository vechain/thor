package types

import (
	"math/big"
)

//Account account
type Account struct {
	Balance *big.Int
	Code    []byte
}
