// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fork

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

var (
	// ErrBaseFeeNotSet is returned if the base fee is not set after the Galactica fork
	ErrBaseFeeNotSet = errors.New("base fee not set after galactica")
	// ErrMaxFeePerGasTooLow is returned if the transaction max fee is less than
	// the base fee of the block.
	ErrMaxFeePerGasTooLow = errors.New("max fee per gas is less than block base fee")
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

func GalacticaOverallGasPrice(tr *tx.Transaction, legacyTxDefaultGasPrice *big.Int, blkBaseFee *big.Int) *big.Int {
	// pow is not accounted for buying gas
	feeItems := GalacticaTxGasPriceAdapter(tr, tr.GasPrice(legacyTxDefaultGasPrice))

	/** This gasPrice is the same that will be used when refunding the user
	* it takes into account the priority fee that will be paid to the validator and the base fee that will be implicitly burned
	* tracked by Energy.TotalAddSub
	*
	* if galactica is not active CurrentBaseFee will be 0
	* LegacyTx GasPrice Calculation = MaxFee
	**/
	currBaseFee := big.NewInt(0)
	if blkBaseFee != nil {
		currBaseFee = blkBaseFee
	}
	return math.BigMin(new(big.Int).Add(feeItems.MaxPriorityFee, currBaseFee), feeItems.MaxFee)
}

func GalacticaPriorityGasPrice(tr *tx.Transaction, legacyTxDefaultGasPrice, provedWork *big.Int, blkBaseFee *big.Int) *big.Int {
	// pow is accounted for priority gas
	feeItems := GalacticaTxGasPriceAdapter(tr, tr.OverallGasPrice(legacyTxDefaultGasPrice, provedWork))

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

func ValidateGalacticaTxFee(tr *tx.Transaction, blockBaseFeeGasPrice, legacyTxDefaultGasPrice *big.Int) error {
	// pow is not accounted for verifying if gas is enough to cover block base fee
	feeItems := GalacticaTxGasPriceAdapter(tr, tr.GasPrice(legacyTxDefaultGasPrice))

	// do not accept txs with less than the block base fee
	if feeItems.MaxFee.Cmp(blockBaseFeeGasPrice) < 0 {
		return fmt.Errorf("%w: expected %s got %s", ErrMaxFeePerGasTooLow, blockBaseFeeGasPrice.String(), feeItems.MaxFee.String())
	}
	return nil
}
