package tx

import (
	"math"
	"math/big"

	"github.com/vechain/thor/thor"
)

var (
	workPerGas = big.NewInt(100)
	big100     = big.NewInt(100)
	big104     = big.NewInt(104) // Moore's law monthly rate (percentage)
	bigE18     = big.NewInt(1e18)
)

// workToGas exchange proved work to gas.
// The decay curve follows Moore's law.
func workToGas(work *big.Int, blockNum uint32) uint64 {
	gas := new(big.Int).Div(work, workPerGas)
	if gas.Sign() == 0 {
		return 0
	}

	months := new(big.Int).SetUint64(uint64(blockNum) * thor.BlockInterval / 3600 / 24 / 30)
	if months.Sign() != 0 {
		x := &big.Int{}
		gas.Mul(gas, x.Exp(big100, months, nil))
		gas.Div(gas, x.Exp(big104, months, nil))
	}

	if gas.BitLen() > 64 {
		return math.MaxUint64
	}
	return gas.Uint64()
}
