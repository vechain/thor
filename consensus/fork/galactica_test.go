// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fork

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
)

func config() *thor.ForkConfig {
	return &thor.ForkConfig{
		GALACTICA: 5,
	}
}

// TestBlockGasLimits tests the gasLimit checks for blocks both across
// the Galactica boundary and post-Galactica blocks
func TestBlockGasLimits(t *testing.T) {
	initial := new(big.Int).SetUint64(thor.InitialBaseFee)

	for i, tc := range []struct {
		pGasLimit uint64
		pNum      uint32
		gasLimit  uint64
		ok        bool
	}{
		// Transitions from non-Galactica to Galactica
		{10000000, 4, 20000000, true},  // No change
		{10000000, 4, 20019531, true},  // Upper limit
		{10000000, 4, 20019532, false}, // Upper +1
		{10000000, 4, 19980469, true},  // Lower limit
		{10000000, 4, 19980468, false}, // Lower limit -1
		// Galactica to Galactica
		{20000000, 5, 20000000, true},
		{20000000, 5, 20019531, true},  // Upper limit
		{20000000, 5, 20019532, false}, // Upper limit +1
		{20000000, 5, 19980469, true},  // Lower limit
		{20000000, 5, 19980468, false}, // Lower limit -1
		{40000000, 5, 40039062, true},  // Upper limit
		{40000000, 5, 40039063, false}, // Upper limit +1
		{40000000, 5, 39960938, true},  // lower limit
		{40000000, 5, 39960937, false}, // Lower limit -1
	} {
		var parentID thor.Bytes32
		binary.BigEndian.PutUint32(parentID[:], tc.pNum-1)

		parent := new(block.Builder).ParentID(parentID).GasUsed(tc.pGasLimit / 2).GasLimit(tc.pGasLimit).BaseFee(initial).Build().Header()
		header := new(block.Builder).ParentID(parent.ID()).GasUsed(tc.gasLimit / 2).GasLimit(tc.gasLimit).BaseFee(initial).Build().Header()
		err := VerifyGalacticaHeader(config(), parent, header)
		if tc.ok && err != nil {
			t.Errorf("test %d: Expected valid header: %s", i, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("test %d: Expected invalid header", i)
		}
	}
}

// TestCalcBaseFee assumes all blocks are post Galactica blocks
func TestCalcBaseFee(t *testing.T) {
	tests := []struct {
		parentBaseFee   int64
		parentGasLimit  uint64
		parentGasUsed   uint64
		expectedBaseFee int64
	}{
		{thor.InitialBaseFee, 20000000, 10000000, thor.InitialBaseFee}, // usage == target
		{thor.InitialBaseFee, 20000000, 9000000, 987500000},            // usage below target
		{thor.InitialBaseFee, 20000000, 11000000, 1012500000},          // usage above target
	}
	for i, test := range tests {
		var parentID thor.Bytes32
		binary.BigEndian.PutUint32(parentID[:], 5)

		parent := new(block.Builder).ParentID(parentID).GasLimit(test.parentGasLimit).GasUsed(test.parentGasUsed).BaseFee(big.NewInt(test.parentBaseFee)).Build().Header()
		if have, want := CalcBaseFee(config(), parent), big.NewInt(test.expectedBaseFee); have.Cmp(want) != 0 {
			t.Errorf("test %d: have %d  want %d, ", i, have, want)
		}
	}
}
