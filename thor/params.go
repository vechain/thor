package thor

import (
	"math/big"

	"github.com/ethereum/go-ethereum/params"
)

// Constants of block chain.
const (
	BlockInterval             uint64 = 10 // time interval between two consecutive blocks.
	ClauseGas                 uint64 = params.TxGas * 2 / 3
	ClauseGasContractCreation uint64 = params.TxGasContractCreation * 2 / 3

	MinGasLimit          uint64 = 1000 * 1000
	InitialGasLimit      uint64 = 10 * 1000 * 1000 // InitialGasLimit gas limit value int genesis block.
	GasLimitBoundDivisor uint64 = 1024             // from ethereum
)

var (
	workEnergyExchangeRate = big.NewInt(1e10) //TODO to be determined
)

// ProvedWorkToEnergy exchange proved work to energy.
// The decay curve follows Moore's law.
func ProvedWorkToEnergy(work *big.Int, blockNum uint32) *big.Int {
	// months past from block 0 to 'blockNum'
	months := new(big.Int).SetUint64(uint64(blockNum) * BlockInterval / 3600 / 24 / 30)

	e := new(big.Int).Mul(work, workEnergyExchangeRate)
	if months.Sign() == 0 {
		return e
	}

	x := big.NewInt(100)
	x.Exp(x, months, nil)

	e.Mul(e, x)

	x.SetInt64(104) // Moore's law monthly rate (percentage)
	x.Exp(x, months, nil)

	return e.Div(e, x)
}
