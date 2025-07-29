// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package galactica

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
)

// CalcBaseFee calculates the base fee of the next block with the given parent block header.
func CalcBaseFee(parent *block.Header, forkConfig *thor.ForkConfig) *big.Int {
	if parent.Number()+1 < forkConfig.GALACTICA {
		return nil
	} else if parent.Number()+1 == forkConfig.GALACTICA {
		// If the current block is the first Galactica block, return the InitialBaseFee.
		return new(big.Int).SetUint64(thor.InitialBaseFee)
	}

	var (
		parentGasTarget          = parent.GasLimit() * thor.GasTargetPercentage / 100
		parentGasTargetBig       = new(big.Int).SetUint64(parentGasTarget)
		baseFeeChangeDenominator = new(big.Int).SetUint64(thor.BaseFeeChangeDenominator)
	)
	parentGasUsed := parent.GasUsed()
	parentBaseFee := parent.BaseFee()

	// If the parent gasUsed is the same as the target, the baseFee remains unchanged.
	if parentGasUsed == parentGasTarget {
		return new(big.Int).Set(parentBaseFee)
	}
	if parentGasUsed > parentGasTarget {
		// If the parent block used more gas than its target, the baseFee should increase.
		// newBaseFee := parentBaseFee + max(1, parentBaseFee * (parentGasUsed - parentGasTarget) / parentGasTarget / baseFeeChangeDenominator)
		gasUsedDelta := new(big.Int).SetUint64(parentGasUsed - parentGasTarget)
		x := new(big.Int).Mul(parentBaseFee, gasUsedDelta)
		// division by zero cannot happen here because of the intrinsic gas pre-check which ensures that tx gas is always
		// greater than 0
		y := x.Div(x, parentGasTargetBig)
		baseFeeDelta := math.BigMax(
			x.Div(y, baseFeeChangeDenominator),
			common.Big1,
		)

		return x.Add(parentBaseFee, baseFeeDelta)
	} else {
		// Otherwise if the parent block used less or equal gas than its target, the baseFee should decrease.
		// newBaseFee := max(InitialBaseFee, parentBaseFee - parentBaseFee * (parentGasTarget - parentGasUsed) / parentGasTarget / baseFeeChangeDenominator)
		gasUsedDelta := new(big.Int).SetUint64(parentGasTarget - parentGasUsed)
		x := new(big.Int).Mul(parentBaseFee, gasUsedDelta)
		y := x.Div(x, parentGasTargetBig)
		baseFeeDelta := x.Div(y, baseFeeChangeDenominator)

		// Setting the minimum baseFee to InitialBaseFee
		return math.BigMax(
			x.Sub(parentBaseFee, baseFeeDelta),
			big.NewInt(thor.InitialBaseFee),
		)
	}
}
