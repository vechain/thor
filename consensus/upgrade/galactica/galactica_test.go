// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package galactica

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
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
		{
			name: "Before galactica fork",
			f: func(t *testing.T) {
				var parentID thor.Bytes32
				binary.BigEndian.PutUint32(parentID[:], 2)

				// parent.Number() = 3
				parent := new(block.Builder).ParentID(parentID).Build().Header()
				baseFee := CalcBaseFee(parent, config())
				assert.Nil(t, baseFee)
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
		parentGasUsed := parentGasLimit * thor.GasTargetPercentage / 100

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
