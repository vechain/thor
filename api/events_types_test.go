// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
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
