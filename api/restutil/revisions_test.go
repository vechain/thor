// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package restutil

import (
	"fmt"
	"math"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestParseRevision(t *testing.T) {
	testCases := []struct {
		revision string
		err      error
		expected *Revision
	}{
		{
			revision: "",
			err:      nil,
			expected: &Revision{revBest},
		},
		{
			revision: "1234",
			err:      nil,
			expected: &Revision{uint32(1234)},
		},
		{
			revision: "best",
			err:      nil,
			expected: &Revision{revBest},
		},
		{
			revision: "justified",
			err:      nil,
			expected: &Revision{revJustified},
		},
		{
			revision: "finalized",
			err:      nil,
			expected: &Revision{revFinalized},
		},
		{
			revision: "next",
			err:      nil,
			expected: &Revision{revNext},
		},
		{
			revision: "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			err:      nil,
			expected: &Revision{thor.MustParseBytes32("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")},
		},
		{
			revision: "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdzz",
			err:      errors.New("encoding/hex: invalid byte: U+007A 'z'"),
			expected: nil,
		},
		{
			revision: "1234567890abcdef1234567890abcde",
			err:      errors.New("strconv.ParseUint: parsing \"1234567890abcdef1234567890abcde\": invalid syntax"),
			expected: nil,
		},
		{
			revision: fmt.Sprintf("%v", uint64(math.MaxUint64)),
			err:      errors.New("block number out of max uint32"),
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.revision, func(t *testing.T) {
			result, err := ParseRevision(tc.revision, true)
			if tc.err != nil {
				assert.Equal(t, tc.err.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestAllowNext(t *testing.T) {
	_, err := ParseRevision("next", false)
	assert.Error(t, err, "invalid revision: next is not allowed")

	_, err = ParseRevision("next", true)
	assert.Nil(t, err)
	_, err = ParseRevision("finalized", false)
	assert.Nil(t, err)
}

func TestGetSummary(t *testing.T) {
	forks := thor.ForkConfig{
		VIP191:    0,
		ETH_CONST: 0,
		BLOCKLIST: 0,
		ETH_IST:   0,
		VIP214:    0,
		FINALITY:  0,
		GALACTICA: 100,
	}
	thorChain, err := testchain.NewWithFork(&forks)
	require.NoError(t, err)

	customRevision := thorChain.Repo().BestBlockSummary().Header.ID()
	// Test cases
	testCases := []struct {
		name     string
		revision *Revision
		err      error
	}{
		{
			name:     "best",
			revision: &Revision{revBest},
			err:      nil,
		},
		{
			name:     "1234",
			revision: &Revision{uint32(1234)},
			err:      errors.New("not found"),
		},
		{
			name:     "justified",
			revision: &Revision{revJustified},
			err:      nil,
		},
		{
			name:     "finalized",
			revision: &Revision{revFinalized},
			err:      nil,
		},
		{
			name:     "customRevision",
			revision: &Revision{customRevision},
			err:      nil,
		},
		{
			name:     "next",
			revision: &Revision{revNext},
			err:      errors.New("invalid revision"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			summary, err := GetSummary(tc.revision, thorChain.Repo(), thorChain.Engine())
			if tc.err != nil {
				assert.Equal(t, tc.err.Error(), err.Error())
			} else {
				assert.Nil(t, err)
				assert.NotNil(t, summary)
			}
		})
	}
}

func TestGetSummaryAndState(t *testing.T) {
	thorChain, err := testchain.NewDefault()
	require.NoError(t, err)

	b := thorChain.GenesisBlock()

	summary, _, err := GetSummaryAndState(&Revision{revBest}, thorChain.Repo(), thorChain.Engine(), thorChain.Stater(), thorChain.GetForkConfig(), false)
	assert.Nil(t, err)
	assert.Equal(t, summary.Header.Number(), b.Header().Number())
	assert.Equal(t, summary.Header.Timestamp(), b.Header().Timestamp())

	summary, _, err = GetSummaryAndState(&Revision{revNext}, thorChain.Repo(), thorChain.Engine(), thorChain.Stater(), thorChain.GetForkConfig(), false)
	assert.Nil(t, err)
	assert.Equal(t, summary.Header.Number(), b.Header().Number()+1)
	assert.Equal(t, summary.Header.Timestamp(), b.Header().Timestamp()+thor.BlockInterval)
	assert.Equal(t, summary.Header.GasUsed(), uint64(0))
	assert.Equal(t, summary.Header.ReceiptsRoot(), tx.Receipts{}.RootHash())
	assert.Equal(t, len(summary.Txs), 0)

	signer, err := summary.Header.Signer()
	assert.NotNil(t, err)
	assert.True(t, signer.IsZero())
}

func TestGetSummaryAndState_PrunedBlock(t *testing.T) {
	t.Run("pruner disabled allows any block", func(t *testing.T) {
		thorChain, err := testchain.NewDefault()
		require.NoError(t, err)

		// Add a few blocks
		account := genesis.DevAccounts()[0]
		for i := 0; i < 10; i++ {
			require.NoError(t, thorChain.MintBlock(account))
		}

		// With pruner disabled, genesis block should be accessible
		revision := &Revision{uint32(0)}
		summary, state, err := GetSummaryAndState(
			revision,
			thorChain.Repo(),
			thorChain.Engine(),
			thorChain.Stater(),
			thorChain.GetForkConfig(),
			true, // pruner disabled
		)

		assert.NoError(t, err)
		assert.NotNil(t, summary)
		assert.NotNil(t, state)
		assert.Equal(t, uint32(0), summary.Header.Number())
	})

	t.Run("pruner enabled allows recent blocks", func(t *testing.T) {
		thorChain, err := testchain.NewDefault()
		require.NoError(t, err)

		// Add a few blocks
		account := genesis.DevAccounts()[0]
		for i := 0; i < 10; i++ {
			require.NoError(t, thorChain.MintBlock(account))
		}

		bestBlock := thorChain.Repo().BestBlockSummary()

		// Recent block should be accessible
		revision := &Revision{bestBlock.Header.Number()}
		summary, state, err := GetSummaryAndState(
			revision,
			thorChain.Repo(),
			thorChain.Engine(),
			thorChain.Stater(),
			thorChain.GetForkConfig(),
			false, // pruner enabled
		)

		assert.NoError(t, err)
		assert.NotNil(t, summary)
		assert.NotNil(t, state)
	})

	t.Run("pruner enabled rejects blocks outside MaxStateHistory", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping block creation test in short mode (takes ~20s)")
		}

		thorChain, err := testchain.NewDefault()
		require.NoError(t, err)

		// Create enough blocks to exceed MaxStateHistory
		// We need bestBlock > MaxStateHistory (65535) to trigger the validation
		account := genesis.DevAccounts()[0]
		numBlocks := thor.MaxStateHistory + 100 // 65635 blocks total (including genesis)

		t.Logf("Creating %d blocks (this takes ~20 seconds)...", numBlocks)
		for i := 0; i < numBlocks; i++ {
			if err := thorChain.MintBlock(account); err != nil {
				t.Fatalf("Failed to create block %d: %v", i, err)
			}
		}

		bestBlock := thorChain.Repo().BestBlockSummary()
		bestBlockNumber := bestBlock.Header.Number()
		oldestAvailable := bestBlockNumber - thor.MaxStateHistory

		t.Logf("Best block: %d, oldest available: %d", bestBlockNumber, oldestAvailable)

		// Test 1: Block within range should be accessible
		blockWithinRange := oldestAvailable + 10
		revision := &Revision{blockWithinRange}
		summary, state, err := GetSummaryAndState(
			revision,
			thorChain.Repo(),
			thorChain.Engine(),
			thorChain.Stater(),
			thorChain.GetForkConfig(),
			false, // pruner enabled
		)

		assert.NoError(t, err)
		assert.NotNil(t, summary)
		assert.NotNil(t, state)
		assert.Equal(t, blockWithinRange, summary.Header.Number())

		// Test 2: Block outside range should return error
		blockOutsideRange := oldestAvailable - 1
		revision = &Revision{blockOutsideRange}
		summary, state, err = GetSummaryAndState(
			revision,
			thorChain.Repo(),
			thorChain.Engine(),
			thorChain.Stater(),
			thorChain.GetForkConfig(),
			false, // pruner enabled
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "state inaccurate")
		assert.Contains(t, err.Error(), "outside the available range")
		assert.Nil(t, summary)
		assert.Nil(t, state)

		// Test 3: Same block outside range should be accessible with pruner disabled
		summary, state, err = GetSummaryAndState(
			revision,
			thorChain.Repo(),
			thorChain.Engine(),
			thorChain.Stater(),
			thorChain.GetForkConfig(),
			true, // pruner disabled
		)

		assert.NoError(t, err)
		assert.NotNil(t, summary)
		assert.NotNil(t, state)
		assert.Equal(t, blockOutsideRange, summary.Header.Number())
	})
}
