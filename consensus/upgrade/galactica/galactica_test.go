// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package galactica

import (
	"encoding/binary"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func config() *thor.ForkConfig {
	return &thor.ForkConfig{
		GALACTICA: 5,
	}
}

// TestCalcBaseFee assumes all blocks are post Galactica blocks
func TestCalcBaseFee(t *testing.T) {
	startingBaseFee := int64(thor.InitialBaseFee * 10)
	tests := []struct {
		parentBaseFee   int64
		parentGasLimit  uint64
		parentGasUsed   uint64
		expectedBaseFee int64
	}{
		{startingBaseFee, 20_000_000, 15_000_000, startingBaseFee},     // usage == target
		{startingBaseFee, 20_000_000, 14_000_000, 99_166_666_666_667},  // usage below target
		{startingBaseFee, 20_000_000, 16_000_000, 100_833_333_333_333}, // usage above target
		{startingBaseFee, 20_000_000, 0, 87_500_000_000_000},           // empty block
	}
	for i, test := range tests {
		var parentID thor.Bytes32
		binary.BigEndian.PutUint32(parentID[:], 5)

		parent := new(block.Builder).ParentID(parentID).GasLimit(test.parentGasLimit).GasUsed(test.parentGasUsed).BaseFee(big.NewInt(test.parentBaseFee)).Build().Header()
		if have, want := CalcBaseFee(parent, config()), big.NewInt(test.expectedBaseFee); have.Cmp(want) != 0 {
			t.Errorf("test %d: have %d  want %d, ", i, have, want)
		}
	}
}

func TestCalcBaseFeeEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		f    func(*testing.T)
	}{
		{
			name: "First galactica block",
			f: func(t *testing.T) {
				var parentID thor.Bytes32
				binary.BigEndian.PutUint32(parentID[:], 3)

				parent := new(block.Builder).ParentID(parentID).Build().Header()
				baseFee := CalcBaseFee(parent, config())
				assert.True(t, baseFee.Cmp(big.NewInt(thor.InitialBaseFee)) == 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.f)
	}
}

func TestBaseFeeLowerBound(t *testing.T) {
	// Post Galactica fork
	var parentID thor.Bytes32
	binary.BigEndian.PutUint32(parentID[:], 5)
	parentGasLimit := uint64(20000000)
	parentGasUsed := uint64(0)
	// Setting the parentBaseFee exactly at 12.5% more, expecting the next base fee to be at the InitialBaseFee level
	parentBaseFee := big.NewInt(thor.InitialBaseFee * 1.125)

	// Generate new block with no gas utilization
	parent := new(block.Builder).ParentID(parentID).GasLimit(parentGasLimit).GasUsed(parentGasUsed).BaseFee(parentBaseFee).Build().Header()
	baseFee := CalcBaseFee(parent, config())
	assert.True(t, baseFee.Cmp(big.NewInt(thor.InitialBaseFee)) == 0)

	// Generate new block again with no gas utitlization
	parent = new(block.Builder).ParentID(parent.ID()).GasLimit(parentGasLimit).GasUsed(parentGasUsed).BaseFee(baseFee).Build().Header()
	baseFee = CalcBaseFee(parent, config())
	assert.True(t, baseFee.Cmp(big.NewInt(thor.InitialBaseFee)) == 0)
}

func TestBaseFeeLimits(t *testing.T) {
	t.Run("EmptyBlocks", func(t *testing.T) {
		// Post Galactica fork
		var parentID thor.Bytes32
		binary.BigEndian.PutUint32(parentID[:], 5)
		parentGasLimit := uint64(20000000)
		parentGasUsed := uint64(0)
		targetDelta := float32(0.875)

		tests := []struct {
			name            string
			blockRange      int
			startingBaseFee *big.Int
		}{
			{
				name:            "short",
				blockRange:      10,
				startingBaseFee: big.NewInt(thor.InitialBaseFee * 10),
			},
			{
				name:            "medium",
				blockRange:      50,
				startingBaseFee: big.NewInt(thor.InitialBaseFee * 1000),
			},
			{
				name:            "long",
				blockRange:      100,
				startingBaseFee: new(big.Int).Mul(big.NewInt(thor.InitialBaseFee*100000), big.NewInt(10000)),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				parentBaseFee := tt.startingBaseFee
				for range tt.blockRange {
					parent := new(block.Builder).ParentID(parentID).GasLimit(parentGasLimit).GasUsed(parentGasUsed).BaseFee(parentBaseFee).Build().Header()
					parentID = parent.ID()
					baseFee := CalcBaseFee(parent, config())

					currentFloat, previousFloat := new(big.Float).SetInt(baseFee), new(big.Float).SetInt(parentBaseFee)
					delta := new(big.Float).Quo(currentFloat, previousFloat)
					deltaFloat, _ := delta.Float32()

					if deltaFloat != targetDelta {
						t.Errorf("delta: %f, targetDelta: %f", delta, targetDelta)
					}
					parentBaseFee = baseFee
				}
			})
		}
	})

	t.Run("FullBlocks", func(t *testing.T) {
		// Post Galactica fork
		var parentID thor.Bytes32
		binary.BigEndian.PutUint32(parentID[:], 5)
		parentGasLimit := uint64(20000000)
		parentGasUsed := uint64(20000000)
		targetDelta := float32(1.0416666)

		tests := []struct {
			name       string
			blockRange int
		}{
			{
				name:       "short",
				blockRange: 10,
			},
			{
				name:       "medium",
				blockRange: 50,
			},
			{
				name:       "long",
				blockRange: 100,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				parentBaseFee := big.NewInt(thor.InitialBaseFee)
				for range tt.blockRange {
					parent := new(block.Builder).ParentID(parentID).GasLimit(parentGasLimit).GasUsed(parentGasUsed).BaseFee(parentBaseFee).Build().Header()
					parentID = parent.ID()
					baseFee := CalcBaseFee(parent, config())

					currentFloat, previousFloat := new(big.Float).SetInt(baseFee), new(big.Float).SetInt(parentBaseFee)
					delta := new(big.Float).Quo(currentFloat, previousFloat)
					deltaFloat, _ := delta.Float32()

					if deltaFloat != targetDelta {
						t.Errorf("delta: %f, targetDelta: %f", delta, targetDelta)
					}
					parentBaseFee = baseFee
				}
			})
		}
	})

	t.Run("Blocks used only halfed, baseFee remains unchanged", func(t *testing.T) {
		// Post Galactica fork
		var parentID thor.Bytes32
		binary.BigEndian.PutUint32(parentID[:], 5)
		parentGasLimit := uint64(20000000)
		parentGasUsed := parentGasLimit * thor.ElasticityMultiplierNum / thor.ElasticityMultiplierDen

		parentBaseFee := big.NewInt(thor.InitialBaseFee * 10)
		for range 100 {
			parent := new(block.Builder).ParentID(parentID).GasLimit(parentGasLimit).GasUsed(parentGasUsed).BaseFee(parentBaseFee).Build().Header()
			parentID = parent.ID()
			baseFee := CalcBaseFee(parent, config())

			assert.True(t, baseFee.Cmp(parentBaseFee) == 0)

			parentBaseFee = baseFee
		}
	})
}

func TestGalacticaGasPrice(t *testing.T) {
	baseGasPrice := big.NewInt(1_000_000_000)
	baseFee := big.NewInt(20_000_000)
	legacyTr := tx.NewBuilder(tx.TypeLegacy).GasPriceCoef(255).Build()

	tests := []struct {
		name string
		f    func(*testing.T)
	}{
		{
			name: "galactica is not yet activated",
			f: func(t *testing.T) {
				res := GalacticaOverallGasPrice(legacyTr, baseGasPrice, nil)
				assert.True(t, res.Cmp(legacyTr.GasPrice(baseGasPrice)) == 0)
			},
		},
		{
			name: "galactica is activated",
			f: func(t *testing.T) {
				res := GalacticaOverallGasPrice(legacyTr, baseGasPrice, baseFee)
				assert.True(t, res.Cmp(legacyTr.GasPrice(baseGasPrice)) == 0)
			},
		},
		{
			name: "galactica is activated, dynamic fee transaction with maxPriorityFee+baseFee as price",
			f: func(t *testing.T) {
				tr := tx.NewBuilder(tx.TypeDynamicFee).MaxFeePerGas(big.NewInt(250_000_000)).MaxPriorityFeePerGas(big.NewInt(15_000)).Build()
				res := GalacticaOverallGasPrice(tr, baseGasPrice, baseFee)
				expectedRes := new(big.Int).Add(tr.MaxPriorityFeePerGas(), baseFee)
				assert.True(t, res.Cmp(expectedRes) == 0)
			},
		},
		{
			name: "galactica is activated, dynamic fee transaction with maxFee as price",
			f: func(t *testing.T) {
				tr := tx.NewBuilder(tx.TypeDynamicFee).MaxFeePerGas(big.NewInt(20_500_000)).MaxPriorityFeePerGas(big.NewInt(1_000_000)).Build()
				res := GalacticaOverallGasPrice(tr, baseGasPrice, baseFee)
				assert.True(t, res.Cmp(tr.MaxFeePerGas()) == 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.f(t)
		})
	}
}

func TestGalacticaPriorityPrice(t *testing.T) {
	baseGasPrice := big.NewInt(1_000_000_000)
	baseFee := big.NewInt(20_000_000)
	provedWork := big.NewInt(100_000)
	legacyTr := tx.NewBuilder(tx.TypeLegacy).Gas(100).GasPriceCoef(255).BlockRef(tx.NewBlockRef(100)).Build()

	tests := []struct {
		name string
		f    func(*testing.T)
	}{
		{
			name: "galactica is not yet activated, use PoW GasPrice",
			f: func(t *testing.T) {
				res := GalacticaPriorityGasPrice(legacyTr, baseGasPrice, provedWork, nil)
				assert.True(t, res.Cmp(legacyTr.OverallGasPrice(baseGasPrice, provedWork)) == 0)
			},
		},
		{
			name: "galactica is not yet activated, do not use base GasPrice for priority",
			f: func(t *testing.T) {
				res := GalacticaPriorityGasPrice(legacyTr, baseGasPrice, provedWork, nil)
				assert.False(t, res.Cmp(legacyTr.GasPrice(baseGasPrice)) == 0)
			},
		},
		{
			name: "galactica is activated",
			f: func(t *testing.T) {
				res := GalacticaPriorityGasPrice(legacyTr, baseGasPrice, provedWork, baseFee)
				expected := new(big.Int).Sub(legacyTr.OverallGasPrice(baseGasPrice, provedWork), baseFee)
				assert.True(t, res.Cmp(expected) == 0)
			},
		},
		{
			name: "galactica is activated, dynamic fee transaction with maxPriorityFee as priority fee",
			f: func(t *testing.T) {
				tr := tx.NewBuilder(tx.TypeDynamicFee).Gas(21000).MaxFeePerGas(big.NewInt(250_000_000)).MaxPriorityFeePerGas(big.NewInt(15_000)).Build()
				res := GalacticaPriorityGasPrice(tr, baseGasPrice, provedWork, baseFee)
				assert.True(t, res.Cmp(tr.MaxPriorityFeePerGas()) == 0)
			},
		},
		{
			name: "galactica is activated, dynamic fee transaction with maxFee-baseFee as priority fee",
			f: func(t *testing.T) {
				tr := tx.NewBuilder(tx.TypeDynamicFee).Gas(2100).MaxFeePerGas(big.NewInt(20_500_000)).MaxPriorityFeePerGas(big.NewInt(1_000_000)).Build()
				res := GalacticaPriorityGasPrice(tr, baseGasPrice, provedWork, baseFee)
				expectedRes := new(big.Int).Sub(tr.MaxFeePerGas(), baseFee)
				assert.True(t, res.Cmp(expectedRes) == 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.f(t)
		})
	}
}

func TestCalculateReward(t *testing.T) {
	rewardRatio := thor.InitialRewardRatio
	tests := []struct {
		name           string
		gasUsed        uint64
		rewardGasPrice *big.Int
		isGalactica    bool
		expectedReward *big.Int
	}{
		{
			name:           "Galactica active, full reward",
			gasUsed:        1000,
			rewardGasPrice: big.NewInt(100),
			isGalactica:    true,
			expectedReward: big.NewInt(100000),
		},
		{
			name:           "Galactica inactive, 30% reward",
			gasUsed:        1000,
			rewardGasPrice: big.NewInt(100),
			isGalactica:    false,
			expectedReward: big.NewInt(30000),
		},
		{
			name:           "Galactica active, zero gas used",
			gasUsed:        0,
			rewardGasPrice: big.NewInt(100),
			isGalactica:    true,
			expectedReward: big.NewInt(0),
		},
		{
			name:           "Galactica inactive, zero gas used",
			gasUsed:        0,
			rewardGasPrice: big.NewInt(100),
			isGalactica:    false,
			expectedReward: big.NewInt(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reward := CalculateReward(tt.gasUsed, tt.rewardGasPrice, rewardRatio, tt.isGalactica)
			assert.Equal(t, tt.expectedReward, reward)
		})
	}
}

func TestValidateGalacticaTxFee(t *testing.T) {
	defaultBaseFee := big.NewInt(20_000_000)
	tests := []struct {
		name                 string
		tx                   *tx.Transaction
		legacyTxBaseGasPrice *big.Int
		blkBaseFeeGasPrice   *big.Int
		wantErr              error
	}{
		{
			name:                 "legacy transaction with enough fee",
			tx:                   tx.NewBuilder(tx.TypeLegacy).GasPriceCoef(255).Build(),
			legacyTxBaseGasPrice: defaultBaseFee,
			blkBaseFeeGasPrice:   defaultBaseFee,
			wantErr:              nil,
		},
		{
			name:                 "legacy transaction with not enough fee",
			tx:                   tx.NewBuilder(tx.TypeLegacy).GasPriceCoef(0).Build(),
			legacyTxBaseGasPrice: defaultBaseFee,
			blkBaseFeeGasPrice:   new(big.Int).Add(defaultBaseFee, common.Big1),
			wantErr:              ErrGasPriceTooLowForBlockBase,
		},
		{
			name:                 "legacy transaction with just enough fee",
			tx:                   tx.NewBuilder(tx.TypeLegacy).GasPriceCoef(1).Build(),
			legacyTxBaseGasPrice: defaultBaseFee,
			blkBaseFeeGasPrice:   new(big.Int).Add(defaultBaseFee, common.Big1),
			wantErr:              nil,
		},
		{
			name:                 "dynamic fee transaction with enough fee",
			tx:                   tx.NewBuilder(tx.TypeDynamicFee).MaxFeePerGas(defaultBaseFee).Build(),
			legacyTxBaseGasPrice: defaultBaseFee,
			blkBaseFeeGasPrice:   defaultBaseFee,
			wantErr:              nil,
		},
		{
			name:                 "dynamic fee transaction not with enough fee",
			tx:                   tx.NewBuilder(tx.TypeDynamicFee).MaxFeePerGas(new(big.Int).Sub(defaultBaseFee, common.Big1)).Build(),
			legacyTxBaseGasPrice: defaultBaseFee,
			blkBaseFeeGasPrice:   defaultBaseFee,
			wantErr:              ErrGasPriceTooLowForBlockBase,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGalacticaTxFee(tt.tx, tt.legacyTxBaseGasPrice, tt.blkBaseFeeGasPrice)
			assert.True(t, errors.Is(err, tt.wantErr))
		})
	}
}
