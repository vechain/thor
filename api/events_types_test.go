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

	// Swapped timestamps (from later than to) describe an empty window, so the
	// converted range must be empty rather than an inverted range.
	rng := newRange(TimeRangeType, bestBlock.Header.Timestamp(), genesis.Timestamp())

	convRng, err := ConvertRange(chain.Repo().NewBestChain(), rng)

	assert.NoError(t, err)
	assert.Equal(t, &emptyRange, convRng)
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

	// Cases here focus on what TestEventsTypes (which builds ranges via newRange
	// with non-nil fields) does not cover: the JSON path, omitted-field defaulting,
	// and boundary capping. Explicit from/to ranges are exercised by TestEventsTypes.
	tests := []struct {
		name     string
		json     string
		expected *logdb.Range
	}{
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

		// Block range - omitted-field defaulting and boundary capping.
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
			name:     "block range at MaxBlockNumber",
			json:     `{"unit": "block", "from": 268435455, "to": 268435455}`,
			expected: &logdb.Range{From: uint32(logdb.MaxBlockNumber), To: uint32(logdb.MaxBlockNumber)},
		},
		{
			name:     "block range to exceeds MaxBlockNumber is capped",
			json:     `{"unit": "block", "from": 5, "to": 999999999999}`,
			expected: &logdb.Range{From: 5, To: uint32(logdb.MaxBlockNumber)},
		},

		// Time range - omitted-field defaulting.
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

		// Time range - swapped timestamps describe an empty window.
		{
			name:     "time range with from > to (swapped timestamps) returns empty",
			json:     fmt.Sprintf(`{"unit": "time", "from": %d, "to": %d}`, bestBlock.Timestamp(), genesis.Timestamp()),
			expected: &emptyRange,
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

// TestConvertRange_TimeWindowBetweenBlocks is a regression test for a time
// window that falls entirely between two consecutive blocks. The from timestamp
// rounds up to the next block while the to timestamp rounds down to the previous
// block, so the naive conversion produced an inverted range (fromBlock > toBlock)
// which logdb interpreted as "from that block to the chain tip", returning
// wildly over-broad results. It must instead resolve to an empty range.
func TestConvertRange_TimeWindowBetweenBlocks(t *testing.T) {
	chain := initChain(t)
	bestChain := chain.Repo().NewBestChain()
	genesis := chain.GenesisBlock().Header()

	// Blocks are spaced by thor.BlockInterval seconds, so [genesis+1, genesis+interval-1]
	// contains no block: block 0 is at genesis, block 1 is at genesis+interval.
	from := genesis.Timestamp() + 1
	to := genesis.Timestamp() + thor.BlockInterval() - 1
	require.Less(t, from, to, "test requires a non-empty, valid time window")

	result, err := ConvertRange(bestChain, newRange(TimeRangeType, from, to))
	require.NoError(t, err)
	assert.Equal(t, &emptyRange, result,
		"a time window between two blocks must convert to an empty range, not an inverted one")
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

// TestConvertRange_WithEvents exercises ConvertRange end-to-end against a chain
// with events spread across non-contiguous blocks, verifying that the converted
// range actually selects the expected blocks when fed to logdb.
func TestConvertRange_WithEvents(t *testing.T) {
	chain, err := testchain.NewDefault()
	require.NoError(t, err)

	// Block layout:
	// Block 0: genesis (no events)
	// Blocks 1-2: empty
	// Block 3: transfer tx (event)
	// Block 4: empty
	// Block 5: transfer tx (event)
	// Block 6: empty
	// Block 7: transfer tx (event)
	// Blocks 8-10: empty
	for range 2 {
		require.NoError(t, chain.MintBlock())
	}

	acc := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1].Address
	clause := tx.NewClause(&recipient).WithValue(big.NewInt(1000))

	require.NoError(t, chain.MintClauses(acc, []*tx.Clause{clause}))
	eventBlock1 := chain.Repo().BestBlockSummary().Header.Number()

	require.NoError(t, chain.MintBlock())

	require.NoError(t, chain.MintClauses(acc, []*tx.Clause{clause}))
	eventBlock2 := chain.Repo().BestBlockSummary().Header.Number()

	require.NoError(t, chain.MintBlock())

	require.NoError(t, chain.MintClauses(acc, []*tx.Clause{clause}))
	eventBlock3 := chain.Repo().BestBlockSummary().Header.Number()

	for range 3 {
		require.NoError(t, chain.MintBlock())
	}

	bestChain := chain.Repo().NewBestChain()
	genesisHeader := chain.GenesisBlock().Header()
	bestBlock := chain.Repo().BestBlockSummary().Header

	filterEvents := func(t *testing.T, rng *logdb.Range) []*logdb.Transfer {
		filter := &logdb.TransferFilter{
			Range:   rng,
			Options: &logdb.Options{Limit: 1000},
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
		{
			name:                     "block range gets all events",
			json:                     fmt.Sprintf(`{"unit": "block", "from": 0, "to": %d}`, bestBlock.Number()),
			expectedBlocksWithEvents: []uint32{eventBlock1, eventBlock2, eventBlock3},
		},
		{
			name:                     "block range covering only first event block",
			json:                     fmt.Sprintf(`{"unit": "block", "from": %d, "to": %d}`, eventBlock1, eventBlock1),
			expectedBlocksWithEvents: []uint32{eventBlock1},
		},
		{
			name:                     "block range covering first two event blocks",
			json:                     fmt.Sprintf(`{"unit": "block", "from": %d, "to": %d}`, eventBlock1, eventBlock2),
			expectedBlocksWithEvents: []uint32{eventBlock1, eventBlock2},
		},
		{
			name:                     "block range in gap between transfers returns nothing",
			json:                     fmt.Sprintf(`{"unit": "block", "from": %d, "to": %d}`, eventBlock1+1, eventBlock2-1),
			expectedBlocksWithEvents: nil,
		},
		{
			name:                     "block range from middle event onwards",
			json:                     fmt.Sprintf(`{"unit": "block", "from": %d}`, eventBlock2),
			expectedBlocksWithEvents: []uint32{eventBlock2, eventBlock3},
		},
		{
			name:                     "time range from genesis to best gets all events",
			json:                     fmt.Sprintf(`{"unit": "time", "from": %d, "to": %d}`, genesisHeader.Timestamp(), bestBlock.Timestamp()),
			expectedBlocksWithEvents: []uint32{eventBlock1, eventBlock2, eventBlock3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rng := &Range{}
			require.NoError(t, json.Unmarshal([]byte(tt.json), rng), "failed to unmarshal JSON: %s", tt.json)

			convertedRange, err := ConvertRange(bestChain, rng)
			require.NoError(t, err)

			var gotBlocks []uint32
			for _, ev := range filterEvents(t, convertedRange) {
				if !slices.Contains(gotBlocks, ev.BlockNumber) {
					gotBlocks = append(gotBlocks, ev.BlockNumber)
				}
			}

			assert.Equal(t, tt.expectedBlocksWithEvents, gotBlocks,
				"expected events in blocks %v, got events in blocks %v", tt.expectedBlocksWithEvents, gotBlocks)
		})
	}
}
