// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin_test

import (
	"math/big"
	"testing"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/xenv"
)

func TestPrototypeNativeBalanceAtBlock_PrunerValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping pruner validation test in short mode (takes ~20s)")
	}

	// Create testchain with enough blocks to exceed MaxStateHistory
	thorChain, err := testchain.NewDefault()
	if err != nil {
		t.Fatal(err)
	}

	account := genesis.DevAccounts()[0]
	numBlocks := thor.MaxStateHistory + 100

	t.Logf("Creating %d blocks (takes ~20 seconds)...", numBlocks)
	for range numBlocks {
		if err := thorChain.MintBlock(account); err != nil {
			t.Fatal(err)
		}
	}

	best := thorChain.Repo().BestBlockSummary()
	bestBlockNumber := best.Header.Number()
	oldestAvailable := bestBlockNumber - thor.MaxStateHistory

	t.Logf("Best block: %d, oldest available: %d", bestBlockNumber, oldestAvailable)

	// Use current state for testing
	st := thorChain.Stater().NewState(best.Root())
	rtChain := thorChain.Repo().NewBestChain()

	// Use an address that should have some balance
	acc1 := genesis.DevAccounts()[0].Address

	// Test 1: Pruner enabled - block within range should work
	t.Run("pruner enabled allows blocks within range", func(t *testing.T) {
		rt := runtime.New(rtChain, st, &xenv.BlockContext{
			Number:     bestBlockNumber,
			Time:       best.Header.Timestamp(),
			TotalScore: 1,
		}, thorChain.GetForkConfig(), false) // pruner enabled

		test := &ctest{
			rt:     rt,
			abi:    builtin.Prototype.ABI,
			to:     builtin.Prototype.Address,
			caller: builtin.Prototype.Address,
		}

		// Block within available range should work (returns balance >= 0)
		blockWithinRange := oldestAvailable + 10
		// We just verify it doesn't return an error - the actual balance doesn't matter
		result := test.Case("balance", acc1, big.NewInt(int64(blockWithinRange)))
		result.Assert(t) // Should not error
	})

	// Test 2: Pruner enabled - block outside range should return zero
	t.Run("pruner enabled returns zero for blocks outside range", func(t *testing.T) {
		rt := runtime.New(rtChain, st, &xenv.BlockContext{
			Number:     bestBlockNumber,
			Time:       best.Header.Timestamp(),
			TotalScore: 1,
		}, thorChain.GetForkConfig(), false) // pruner enabled

		test := &ctest{
			rt:     rt,
			abi:    builtin.Prototype.ABI,
			to:     builtin.Prototype.Address,
			caller: builtin.Prototype.Address,
		}

		// Block outside available range should return zero (validation kicks in)
		blockOutsideRange := oldestAvailable - 1
		test.Case("balance", acc1, big.NewInt(int64(blockOutsideRange))).
			ShouldOutput(big.NewInt(0)).
			Assert(t)

		test.Case("energy", acc1, big.NewInt(int64(blockOutsideRange))).
			ShouldOutput(big.NewInt(0)).
			Assert(t)
	})

	// Test 3: Edge case - exactly at the oldest available block
	t.Run("pruner enabled at boundary block", func(t *testing.T) {
		rt := runtime.New(rtChain, st, &xenv.BlockContext{
			Number:     bestBlockNumber,
			Time:       best.Header.Timestamp(),
			TotalScore: 1,
		}, thorChain.GetForkConfig(), false) // pruner enabled

		test := &ctest{
			rt:     rt,
			abi:    builtin.Prototype.ABI,
			to:     builtin.Prototype.Address,
			caller: builtin.Prototype.Address,
		}

		// Exactly at the boundary should work (validation allows it)
		result := test.Case("balance", acc1, big.NewInt(int64(oldestAvailable)))
		result.Assert(t) // Should not error
	})

	// Test 4: Current block should always work
	t.Run("current block always accessible", func(t *testing.T) {
		rt := runtime.New(rtChain, st, &xenv.BlockContext{
			Number:     bestBlockNumber,
			Time:       best.Header.Timestamp(),
			TotalScore: 1,
		}, thorChain.GetForkConfig(), false) // pruner enabled

		test := &ctest{
			rt:     rt,
			abi:    builtin.Prototype.ABI,
			to:     builtin.Prototype.Address,
			caller: builtin.Prototype.Address,
		}

		// Current block should work (uses current state)
		result := test.Case("balance", acc1, big.NewInt(int64(bestBlockNumber)))
		result.Assert(t) // Should not error
	})
}
