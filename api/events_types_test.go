// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func newRange(unit RangeType, from uint64, to uint64) *Range {
	return &Range{
		Unit: unit,
		From: &from,
		To:   &to,
	}
}

func TestEventsTypes(t *testing.T) {
	c := initChain(t)
	for name, tt := range map[string]func(*testing.T, *testchain.Chain){
		"testConvertRangeWithBlockRangeType":                          testConvertRangeWithBlockRangeType,
		"testConvertRangeWithBlockRangeTypeMoreThanMaxBlockNumber":    testConvertRangeWithBlockRangeTypeMoreThanMaxBlockNumber,
		"testConvertRangeWithBlockRangeTypeWithSwitchedFromAndTo":     testConvertRangeWithBlockRangeTypeWithSwitchedFromAndTo,
		"testConvertRangeWithTimeRangeTypeLessThenGenesis":            testConvertRangeWithTimeRangeTypeLessThenGenesis,
		"testConvertRangeWithTimeRangeType":                           testConvertRangeWithTimeRangeType,
		"testConvertRangeWithFromGreaterThanGenesis":                  testConvertRangeWithFromGreaterThanGenesis,
		"testConvertRangeWithTimeRangeLessThanGenesisGreaterThanBest": testConvertRangeWithTimeRangeLessThanGenesisGreaterThanBest,
		"testConvertRangeWithTimeRangeTypeWithSwitchedFromAndTo":      testConvertRangeWithTimeRangeTypeWithSwitchedFromAndTo,
	} {
		t.Run(name, func(t *testing.T) {
			tt(t, c)
		})
	}
}

func testConvertRangeWithTimeRangeLessThanGenesisGreaterThanBest(t *testing.T, chain *testchain.Chain) {
	genesis := chain.GenesisBlock().Header()
	bestBlock := chain.Repo().BestBlockSummary()

	rng := newRange(TimeRangeType, genesis.Timestamp()-1_000, bestBlock.Header.Timestamp()+1_000)
	expectedRange := &logdb.Range{
		From: genesis.Number(),
		To:   bestBlock.Header.Number(),
	}

	convRng, err := ConvertRange(chain.Repo().NewBestChain(), rng)

	assert.NoError(t, err)
	assert.Equal(t, expectedRange, convRng)
}

func testConvertRangeWithTimeRangeTypeWithSwitchedFromAndTo(t *testing.T, chain *testchain.Chain) {
	genesis := chain.GenesisBlock().Header()
	bestBlock := chain.Repo().BestBlockSummary()

	rng := newRange(TimeRangeType, bestBlock.Header.Timestamp(), genesis.Timestamp())
	expectedRange := &logdb.Range{
		From: bestBlock.Header.Number(),
		To:   genesis.Number(),
	}

	convRng, err := ConvertRange(chain.Repo().NewBestChain(), rng)

	assert.NoError(t, err)
	assert.Equal(t, expectedRange, convRng)
}

func testConvertRangeWithBlockRangeType(t *testing.T, chain *testchain.Chain) {
	rng := newRange(BlockRangeType, 1, 2)

	convertedRng, err := ConvertRange(chain.Repo().NewBestChain(), rng)

	assert.NoError(t, err)
	assert.Equal(t, uint32(*rng.From), convertedRng.From)
	assert.Equal(t, uint32(*rng.To), convertedRng.To)
}

func testConvertRangeWithBlockRangeTypeMoreThanMaxBlockNumber(t *testing.T, chain *testchain.Chain) {
	rng := newRange(BlockRangeType, logdb.MaxBlockNumber+1, logdb.MaxBlockNumber+2)

	convertedRng, err := ConvertRange(chain.Repo().NewBestChain(), rng)

	assert.NoError(t, err)
	assert.Equal(t, &emptyRange, convertedRng)
}

func testConvertRangeWithBlockRangeTypeWithSwitchedFromAndTo(t *testing.T, chain *testchain.Chain) {
	rng := newRange(BlockRangeType, logdb.MaxBlockNumber, 0)

	convertedRng, err := ConvertRange(chain.Repo().NewBestChain(), rng)

	assert.NoError(t, err)
	assert.Equal(t, emptyRange.From, convertedRng.From)
	assert.Equal(t, uint32(*rng.To), convertedRng.To)
}

func testConvertRangeWithTimeRangeTypeLessThenGenesis(t *testing.T, chain *testchain.Chain) {
	rng := newRange(TimeRangeType, chain.GenesisBlock().Header().Timestamp()-1000, chain.GenesisBlock().Header().Timestamp()-100)
	expectedEmptyRange := &logdb.Range{
		From: logdb.MaxBlockNumber,
		To:   logdb.MaxBlockNumber,
	}

	convRng, err := ConvertRange(chain.Repo().NewBestChain(), rng)

	assert.NoError(t, err)
	assert.Equal(t, expectedEmptyRange, convRng)
}

func testConvertRangeWithTimeRangeType(t *testing.T, chain *testchain.Chain) {
	genesis := chain.GenesisBlock().Header()

	rng := newRange(TimeRangeType, 1, genesis.Timestamp())
	expectedZeroRange := &logdb.Range{
		From: 0,
		To:   0,
	}

	convRng, err := ConvertRange(chain.Repo().NewBestChain(), rng)

	assert.NoError(t, err)
	assert.Equal(t, expectedZeroRange, convRng)
}

func testConvertRangeWithFromGreaterThanGenesis(t *testing.T, chain *testchain.Chain) {
	genesis := chain.GenesisBlock().Header()

	rng := newRange(TimeRangeType, genesis.Timestamp()+1_000, genesis.Timestamp()+10_000)
	expectedEmptyRange := &logdb.Range{
		From: logdb.MaxBlockNumber,
		To:   logdb.MaxBlockNumber,
	}

	convRng, err := ConvertRange(chain.Repo().NewBestChain(), rng)

	assert.NoError(t, err)
	assert.Equal(t, expectedEmptyRange, convRng)
}

// Init functions
func initChain(t *testing.T) *testchain.Chain {
	thorChain, err := testchain.NewDefault()
	require.NoError(t, err)

	for range 10 {
		require.NoError(t, thorChain.MintBlock())
	}

	return thorChain
}

func TestConvertRange_Matrix(t *testing.T) {
	chain := initChain(t)
	bestChain := chain.Repo().NewBestChain()
	genesis := chain.GenesisBlock().Header()
	bestBlock := chain.Repo().BestBlockSummary().Header

	tests := []struct {
		name        string
		json        string
		expected    *logdb.Range
		expectError bool
	}{
		// Nil/empty JSON
		{
			name:     "null JSON returns nil",
			json:     `null`,
			expected: nil,
		},
		{
			name:     "empty object returns full block range",
			json:     `{}`,
			expected: &logdb.Range{From: 0, To: uint32(logdb.MaxBlockNumber)},
		},

		// Block range type - basic cases
		{
			name:     "block range with from and to",
			json:     `{"unit": "block", "from": 5, "to": 10}`,
			expected: &logdb.Range{From: 5, To: 10},
		},
		{
			name:     "block range with zero from and to",
			json:     `{"unit": "block", "from": 0, "to": 0}`,
			expected: &logdb.Range{From: 0, To: 0},
		},
		{
			name:     "block range with same from and to",
			json:     `{"unit": "block", "from": 100, "to": 100}`,
			expected: &logdb.Range{From: 100, To: 100},
		},

		// Block range type - omitted from/to
		{
			name:     "block range with omitted from defaults to 0",
			json:     `{"unit": "block", "to": 10}`,
			expected: &logdb.Range{From: 0, To: 10},
		},
		{
			name:     "block range with omitted to defaults to MaxBlockNumber",
			json:     `{"unit": "block", "from": 5}`,
			expected: &logdb.Range{From: 5, To: uint32(logdb.MaxBlockNumber)},
		},
		{
			name:     "block range with only unit defaults to full range",
			json:     `{"unit": "block"}`,
			expected: &logdb.Range{From: 0, To: uint32(logdb.MaxBlockNumber)},
		},

		// Block range type - boundary values
		{
			name:     "block range at MaxBlockNumber",
			json:     `{"unit": "block", "from": 268435455, "to": 268435455}`,
			expected: &logdb.Range{From: uint32(logdb.MaxBlockNumber), To: uint32(logdb.MaxBlockNumber)},
		},
		{
			name:     "block range from exceeds MaxBlockNumber returns empty",
			json:     `{"unit": "block", "from": 268435456, "to": 268435457}`,
			expected: &emptyRange,
		},
		{
			name:     "block range to exceeds MaxBlockNumber is capped",
			json:     `{"unit": "block", "from": 5, "to": 999999999999}`,
			expected: &logdb.Range{From: 5, To: uint32(logdb.MaxBlockNumber)},
		},

		// Omitted unit (defaults to block range)
		{
			name:     "omitted unit with from and to behaves like block range",
			json:     `{"from": 5, "to": 10}`,
			expected: &logdb.Range{From: 5, To: 10},
		},
		{
			name:     "omitted unit with only from",
			json:     `{"from": 5}`,
			expected: &logdb.Range{From: 5, To: uint32(logdb.MaxBlockNumber)},
		},
		{
			name:     "omitted unit with only to",
			json:     `{"to": 10}`,
			expected: &logdb.Range{From: 0, To: 10},
		},

		// Time range type - before genesis
		{
			name:     "time range entirely before genesis returns empty",
			json:     fmt.Sprintf(`{"unit": "time", "from": %d, "to": %d}`, genesis.Timestamp()-2000, genesis.Timestamp()-1000),
			expected: &emptyRange,
		},

		// Time range type - after head
		{
			name:     "time range entirely after head returns empty",
			json:     fmt.Sprintf(`{"unit": "time", "from": %d, "to": %d}`, bestBlock.Timestamp()+1000, bestBlock.Timestamp()+2000),
			expected: &emptyRange,
		},

		// Time range type - spanning genesis to head
		{
			name: "time range from before genesis to after head returns full chain",
			json: fmt.Sprintf(`{"unit": "time", "from": %d, "to": %d}`, genesis.Timestamp()-1000, bestBlock.Timestamp()+1000),
			expected: &logdb.Range{
				From: genesis.Number(),
				To:   bestBlock.Number(),
			},
		},

		// Time range type - exact timestamps
		{
			name: "time range at genesis timestamp",
			json: fmt.Sprintf(`{"unit": "time", "from": %d, "to": %d}`, genesis.Timestamp(), genesis.Timestamp()),
			expected: &logdb.Range{
				From: genesis.Number(),
				To:   genesis.Number(),
			},
		},

		// Time range type - omitted from/to
		{
			name: "time range with omitted from defaults to genesis",
			json: fmt.Sprintf(`{"unit": "time", "to": %d}`, bestBlock.Timestamp()),
			expected: &logdb.Range{
				From: genesis.Number(),
				To:   bestBlock.Number(),
			},
		},
		{
			name: "time range with omitted to defaults to head",
			json: fmt.Sprintf(`{"unit": "time", "from": %d}`, genesis.Timestamp()),
			expected: &logdb.Range{
				From: genesis.Number(),
				To:   bestBlock.Number(),
			},
		},
		{
			name: "time range with only unit defaults to full chain",
			json: `{"unit": "time"}`,
			expected: &logdb.Range{
				From: genesis.Number(),
				To:   bestBlock.Number(),
			},
		},

		// Time range type - zero values (explicitly set to 0, not omitted)
		{
			name:     "time range with zero from and to (before any genesis) returns empty",
			json:     `{"unit": "time", "from": 0, "to": 0}`,
			expected: &emptyRange,
		},

		// Block range - from > to (inverted range, no validation in ConvertRange)
		{
			name:     "block range with from > to passes through",
			json:     `{"unit": "block", "from": 100, "to": 50}`,
			expected: &logdb.Range{From: 100, To: 50},
		},

		// Time range - from > to (inverted timestamps)
		{
			name: "time range with from > to (swapped timestamps)",
			json: fmt.Sprintf(`{"unit": "time", "from": %d, "to": %d}`, bestBlock.Timestamp(), genesis.Timestamp()),
			expected: &logdb.Range{
				From: bestBlock.Number(),
				To:   genesis.Number(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rng *Range
			if tt.json != "null" {
				rng = &Range{}
				err := json.Unmarshal([]byte(tt.json), rng)
				require.NoError(t, err, "failed to unmarshal JSON: %s", tt.json)
			}

			result, err := ConvertRange(bestChain, rng)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.From, result.From, "From mismatch")
				assert.Equal(t, tt.expected.To, result.To, "To mismatch")
			}
		})
	}
}

func TestConvertEvent(t *testing.T) {
	event := &logdb.Event{
		Address:     thor.Address{0x01},
		Data:        []byte{0x02, 0x03},
		BlockID:     thor.Bytes32{0x04},
		BlockNumber: 5,
		BlockTime:   6,
		TxID:        thor.Bytes32{0x07},
		TxIndex:     8,
		LogIndex:    9,
		TxOrigin:    thor.Address{0x0A},
		ClauseIndex: 10,
		Topics: [5]*thor.Bytes32{
			{0x0B},
			{0x0C},
			nil,
			nil,
			nil,
		},
	}

	expectedTopics := []*thor.Bytes32{
		{0x0B},
		{0x0C},
	}
	expectedData := hexutil.Encode(event.Data)

	result := ConvertEvent(event, true)

	assert.Equal(t, event.Address, result.Address)
	assert.Equal(t, expectedData, result.Data)
	assert.Equal(t, event.BlockID, result.Meta.BlockID)
	assert.Equal(t, event.BlockNumber, result.Meta.BlockNumber)
	assert.Equal(t, event.BlockTime, result.Meta.BlockTimestamp)
	assert.Equal(t, event.TxID, result.Meta.TxID)
	assert.Equal(t, event.TxIndex, *result.Meta.TxIndex)
	assert.Equal(t, event.LogIndex, *result.Meta.LogIndex)
	assert.Equal(t, event.TxOrigin, result.Meta.TxOrigin)
	assert.Equal(t, event.ClauseIndex, result.Meta.ClauseIndex)
	assert.Equal(t, expectedTopics, result.Topics)
}

// TestConvertRange_WithEvents tests the ConvertRange function with actual events
// to verify that range filtering returns the correct events.
func TestConvertRange_WithEvents(t *testing.T) {
	chain, err := testchain.NewDefault()
	require.NoError(t, err)

	// Block layout:
	// Block 0: genesis (no events)
	// Block 1: empty (no txs)
	// Block 2: empty (no txs)
	// Block 3: tx with event (eventBlock1)
	// Block 4: empty (no txs)
	// Block 5: tx with event (eventBlock2)
	// Block 6: empty (no txs)
	// Block 7: tx with event (eventBlock3)
	// Block 8: empty (no txs)
	// Block 9: empty (no txs)
	// Block 10: empty (no txs)

	// Mint blocks 1-2 (empty)
	for range 2 {
		require.NoError(t, chain.MintBlock())
	}

	// Block 3: mint with a transfer tx (generates Transfer event from Energy contract)
	acc := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1].Address
	clause := tx.NewClause(&recipient).WithValue(big.NewInt(1000))
	require.NoError(t, chain.MintClauses(acc, []*tx.Clause{clause}))
	eventBlock1 := chain.Repo().BestBlockSummary().Header.Number()

	// Block 4: empty
	require.NoError(t, chain.MintBlock())

	// Block 5: mint with another transfer tx
	require.NoError(t, chain.MintClauses(acc, []*tx.Clause{clause}))
	eventBlock2 := chain.Repo().BestBlockSummary().Header.Number()

	// Block 6: empty
	require.NoError(t, chain.MintBlock())

	// Block 7: mint with another transfer tx
	require.NoError(t, chain.MintClauses(acc, []*tx.Clause{clause}))
	eventBlock3 := chain.Repo().BestBlockSummary().Header.Number()

	// Blocks 8-10: empty
	for range 3 {
		require.NoError(t, chain.MintBlock())
	}

	bestChain := chain.Repo().NewBestChain()
	genesisHeader := chain.GenesisBlock().Header()
	bestBlock := chain.Repo().BestBlockSummary().Header

	// Helper to filter events with a given range
	filterEvents := func(t *testing.T, rng *logdb.Range) []*logdb.Transfer {
		limit := uint64(1000)
		filter := &logdb.TransferFilter{
			Range:   rng,
			Options: &logdb.Options{Limit: limit},
		}
		events, err := chain.LogDB().FilterTransfers(context.Background(), filter)
		require.NoError(t, err)
		return events
	}

	tests := []struct {
		name                     string
		json                     string
		expectedBlocksWithEvents []uint32
	}{
		// Full range - should get all events
		{
			name:                     "empty object gets all events",
			json:                     `{}`,
			expectedBlocksWithEvents: []uint32{eventBlock1, eventBlock2, eventBlock3},
		},

		// Block range covering all event blocks
		{
			name:                     "block range 0 to best gets all events",
			json:                     fmt.Sprintf(`{"unit": "block", "from": 0, "to": %d}`, bestBlock.Number()),
			expectedBlocksWithEvents: []uint32{eventBlock1, eventBlock2, eventBlock3},
		},

		// Block range covering only first event
		{
			name:                     "block range covering only first event block",
			json:                     fmt.Sprintf(`{"unit": "block", "from": %d, "to": %d}`, eventBlock1, eventBlock1),
			expectedBlocksWithEvents: []uint32{eventBlock1},
		},

		// Block range covering middle event only
		{
			name:                     "block range covering only middle event block",
			json:                     fmt.Sprintf(`{"unit": "block", "from": %d, "to": %d}`, eventBlock2, eventBlock2),
			expectedBlocksWithEvents: []uint32{eventBlock2},
		},

		// Block range covering first two events
		{
			name:                     "block range covering first two event blocks",
			json:                     fmt.Sprintf(`{"unit": "block", "from": %d, "to": %d}`, eventBlock1, eventBlock2),
			expectedBlocksWithEvents: []uint32{eventBlock1, eventBlock2},
		},

		// Block range before any transfers
		{
			name:                     "block range before any transfers",
			json:                     fmt.Sprintf(`{"unit": "block", "from": 0, "to": %d}`, eventBlock1-1),
			expectedBlocksWithEvents: nil,
		},

		// Block range after all transfers
		{
			name:                     "block range after all transfers",
			json:                     fmt.Sprintf(`{"unit": "block", "from": %d, "to": %d}`, eventBlock3+1, bestBlock.Number()),
			expectedBlocksWithEvents: nil,
		},

		// Block range with gaps (between transfers)
		{
			name:                     "block range in gap between transfers",
			json:                     fmt.Sprintf(`{"unit": "block", "from": %d, "to": %d}`, eventBlock1+1, eventBlock2-1),
			expectedBlocksWithEvents: nil,
		},

		// Time range covering all events
		{
			name:                     "time range from genesis to best gets all events",
			json:                     fmt.Sprintf(`{"unit": "time", "from": %d, "to": %d}`, genesisHeader.Timestamp(), bestBlock.Timestamp()),
			expectedBlocksWithEvents: []uint32{eventBlock1, eventBlock2, eventBlock3},
		},

		// Time range with omitted from (defaults to genesis)
		{
			name:                     "time range with omitted from gets all events",
			json:                     fmt.Sprintf(`{"unit": "time", "to": %d}`, bestBlock.Timestamp()),
			expectedBlocksWithEvents: []uint32{eventBlock1, eventBlock2, eventBlock3},
		},

		// Time range with omitted to (defaults to head)
		{
			name:                     "time range with omitted to gets all events",
			json:                     fmt.Sprintf(`{"unit": "time", "from": %d}`, genesisHeader.Timestamp()),
			expectedBlocksWithEvents: []uint32{eventBlock1, eventBlock2, eventBlock3},
		},

		// Block range with omitted from (defaults to 0)
		{
			name:                     "block range with omitted from gets all events",
			json:                     fmt.Sprintf(`{"unit": "block", "to": %d}`, bestBlock.Number()),
			expectedBlocksWithEvents: []uint32{eventBlock1, eventBlock2, eventBlock3},
		},

		// Block range with omitted to (defaults to MaxBlockNumber)
		{
			name:                     "block range with omitted to gets all events from start",
			json:                     `{"unit": "block", "from": 0}`,
			expectedBlocksWithEvents: []uint32{eventBlock1, eventBlock2, eventBlock3},
		},

		// Block range starting from middle
		{
			name:                     "block range from middle event onwards",
			json:                     fmt.Sprintf(`{"unit": "block", "from": %d}`, eventBlock2),
			expectedBlocksWithEvents: []uint32{eventBlock2, eventBlock3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rng *Range
			if tt.json != "null" && tt.json != "" {
				rng = &Range{}
				err := json.Unmarshal([]byte(tt.json), rng)
				require.NoError(t, err, "failed to unmarshal JSON: %s", tt.json)
			}

			convertedRange, err := ConvertRange(bestChain, rng)
			require.NoError(t, err)

			events := filterEvents(t, convertedRange)

			// Extract block numbers from events
			var gotBlocks []uint32
			for _, ev := range events {
				// Only add if not already in the list (multiple events per block possible)
				found := slices.Contains(gotBlocks, ev.BlockNumber)
				if !found {
					gotBlocks = append(gotBlocks, ev.BlockNumber)
				}
			}

			assert.Equal(t, tt.expectedBlocksWithEvents, gotBlocks,
				"expected events in blocks %v, got events in blocks %v", tt.expectedBlocksWithEvents, gotBlocks)
		})
	}
}
