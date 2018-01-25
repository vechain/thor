package tx

import (
	"math/big"

	"github.com/vechain/thor/thor"
)

var (
	bigMaxTxWorkDelay = new(big.Int).SetUint64(uint64(thor.MaxTxWorkDelay))
	big100            = big.NewInt(100)
	big104            = big.NewInt(104) // Moore's law monthly rate (percentage)
)

// ProvedWorkToEnergy exchange proved work to energy.
// 'blockNum' is used to calculate exchange rate.
/// 'delay' is used to decay.
// The decay curve follows Moore's law.
func ProvedWorkToEnergy(work *big.Int, blockNum, delay uint32) *big.Int {
	if delay >= thor.MaxTxWorkDelay || work.Sign() == 0 {
		return &big.Int{}
	}

	// months past from block 0 to 'blockNum'
	months := new(big.Int).SetUint64(uint64(blockNum) * thor.BlockInterval / 3600 / 24 / 30)

	energy := &big.Int{}
	energy.Mul(work, thor.WorkEnergyExchangeRate)
	x := &big.Int{}

	if months.Sign() != 0 {
		energy.Mul(energy, x.Exp(big100, months, nil))
		energy.Div(energy, x.Exp(big104, months, nil))
	}

	// decay by delay
	energy.Mul(energy, x.SetUint64(uint64(thor.MaxTxWorkDelay-delay)))
	return energy.Div(energy, bigMaxTxWorkDelay)
}
