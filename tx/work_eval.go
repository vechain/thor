// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"math"
	"math/big"

	"github.com/vechain/thor/thor"
)

var (
	workPerGas = big.NewInt(1000)
	big100     = big.NewInt(100)
	big104     = big.NewInt(104) // Moore's law monthly rate (percentage)
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
