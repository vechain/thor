// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fork

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
)

// VerifyGalacticaHeader verifies some header attributes which were changed in Galactica fork,
// - gas limit check
// - basefee check
func VerifyGalacticaHeader(config *thor.ForkConfig, parent, header *block.Header) error {
	// Verify the header is not malformed
	if header.BaseFee() == nil {
		return fmt.Errorf("header is missing baseFee")
	}

	// Verify that the gas limit remains within allowed bounds
	parentGasLimit := parent.GasLimit()
	if err := block.GasLimit(header.GasLimit()).IsValid(parentGasLimit); !err {
		return fmt.Errorf("invalid gas limit: have %d, want %d", header.GasLimit(), parentGasLimit)
	}

	// Verify the baseFee is correct based on the parent header.
	expectedBaseFee := CalcBaseFee(config, parent)
	if header.BaseFee().Cmp(expectedBaseFee) != 0 {
		return fmt.Errorf("invalid baseFee: have %s, want %s, parentBaseFee %s, parentGasUsed %d",
			expectedBaseFee, header.BaseFee(), parent.BaseFee(), parent.GasUsed())
	}
	return nil
}

// CalcBaseFee calculates the basefee of the header.
func CalcBaseFee(config *thor.ForkConfig, parent *block.Header) *big.Int {
	// If the current block is the first Galactica block, return the InitialBaseFee.
	if parent.Number()+1 == config.GALACTICA {
		return new(big.Int).SetUint64(thor.InitialBaseFee)
	}

	var (
		parentGasTarget          = parent.GasLimit() / thor.ElasticityMultiplier
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
		y := x.Div(x, parentGasTargetBig)
		baseFeeDelta := math.BigMax(
			x.Div(y, baseFeeChangeDenominator),
			common.Big1,
		)

		return x.Add(parentBaseFee, baseFeeDelta)
	} else {
		// Otherwise if the parent block used less or equal gas than its target, the baseFee should decrease.
		// newBaseFee := max(0, parentBaseFee - parentBaseFee * (parentGasTarget - parentGasUsed) / parentGasTarget / baseFeeChangeDenominator)
		gasUsedDelta := new(big.Int).SetUint64(parentGasTarget - parentGasUsed)
		x := new(big.Int).Mul(parentBaseFee, gasUsedDelta)
		y := x.Div(x, parentGasTargetBig)
		baseFeeDelta := x.Div(y, baseFeeChangeDenominator)

		return math.BigMax(
			x.Sub(parentBaseFee, baseFeeDelta),
			common.Big0,
		)
	}
}
