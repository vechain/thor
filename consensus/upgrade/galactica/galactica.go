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
	"github.com/vechain/thor/v2/tx"
)

// CalcBaseFee calculates the basefee of the header.
func CalcBaseFee(parent *block.Header, forkConfig *thor.ForkConfig) *big.Int {
	if parent.Number()+1 < forkConfig.GALACTICA {
		return nil
	} else if parent.Number()+1 == forkConfig.GALACTICA {
		// If the current block is the first Galactica block, return the InitialBaseFee.
		return new(big.Int).SetUint64(thor.InitialBaseFee)
	}

	var (
		parentGasTarget          = parent.GasLimit() * thor.ElasticityMultiplierNum / thor.ElasticityMultiplierDen
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

func GalacticaTxGasPriceAdapter(tr *tx.Transaction, legacyTxFinalGasPrice *big.Int) *GalacticaFeeMarketItems {
	var maxPriorityFee, maxFee *big.Int
	switch tr.Type() {
	case tx.TypeLegacy:
		maxFee = legacyTxFinalGasPrice
		maxPriorityFee = legacyTxFinalGasPrice
	case tx.TypeDynamicFee:
		maxPriorityFee = tr.MaxPriorityFeePerGas()
		maxFee = tr.MaxFeePerGas()
	}
	return &GalacticaFeeMarketItems{maxFee, maxPriorityFee}
}

type GalacticaFeeMarketItems struct {
	MaxFee         *big.Int
	MaxPriorityFee *big.Int
}

func GalacticaPriorityGasPrice(tr *tx.Transaction, legacyTxBaseGasPrice, provedWork *big.Int, blkBaseFee *big.Int) *big.Int {
	// proved work is accounted for priority gas
	feeItems := GalacticaTxGasPriceAdapter(tr, tr.OverallGasPrice(legacyTxBaseGasPrice, provedWork))

	/** This gasPrice will be used to compensate the validator
	* baseFee=1000; maxFee = 1000; maxPriorityFee = 100 -> validator gets  0
	* baseFee=900;  maxFee = 1000; maxPriorityFee = 0   -> validator get   0; user gets back 100
	* baseFee=900;  maxFee = 1000; maxPriorityFee = 50  -> validator gets 50
	* baseFee=1100; maxFee = 1000; maxPriorityFee = 100 -> tx rejected, maxFee < baseFee
	*
	* if galactica is not active CurrentBaseFee will be 0
	* LegacyTx OverallGasPrice Calculation = MaxFee
	**/
	currBaseFee := big.NewInt(0)
	if blkBaseFee != nil {
		currBaseFee = blkBaseFee
	}
	return math.BigMin(feeItems.MaxPriorityFee, new(big.Int).Sub(feeItems.MaxFee, currBaseFee))
}

func CalculateReward(gasUsed uint64, rewardGasPrice, rewardRatio *big.Int, isGalactica bool) *big.Int {
	reward := new(big.Int).SetUint64(gasUsed)
	reward.Mul(reward, rewardGasPrice)
	if isGalactica {
		return reward
	}
	// Returning the 30% of the reward
	reward.Mul(reward, rewardRatio)
	reward.Div(reward, big.NewInt(1e18))
	return reward
}
