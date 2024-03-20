// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package events

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

func TestEventsTypes(t *testing.T) {
	chain := initChain(t)

	testConvertRangeWithBlockRangeType(t, chain)
	testConvertRangeWithTimeRangeTypeLessThenGenesis(t, chain)
	testConvertRangeWithTimeRangeType(t, chain)
	testMultipleConvertEventFilters(t, chain)
	testMultipleConvertEvent(t)
	testConvertRangeWithFromGreaterThanGenesis(t, chain)
}

func testConvertRangeWithBlockRangeType(t *testing.T, chain *chain.Chain) {
	rng := &Range{
		Unit: BlockRangeType,
		From: 1,
		To:   2,
	}

	convertedRng, err := ConvertRange(chain, rng)

	assert.NoError(t, err)
	assert.Equal(t, uint32(rng.From), convertedRng.From)
	assert.Equal(t, uint32(rng.To), convertedRng.To)
}

func testConvertRangeWithTimeRangeTypeLessThenGenesis(t *testing.T, chain *chain.Chain) {
	rng := &Range{
		Unit: TimeRangeType,
		From: 1,
		To:   2,
	}
	expectedEmptyRange := &logdb.Range{
		From: math.MaxUint32,
		To:   math.MaxUint32,
	}

	convRng, err := ConvertRange(chain, rng)

	assert.NoError(t, err)
	assert.Equal(t, expectedEmptyRange, convRng)
}

func testConvertRangeWithTimeRangeType(t *testing.T, chain *chain.Chain) {
	genesis, err := chain.GetBlockHeader(0)
	if err != nil {
		t.Fatal(err)
	}
	rng := &Range{
		Unit: TimeRangeType,
		From: 1,
		To:   genesis.Timestamp(),
	}
	expectedZeroRange := &logdb.Range{
		From: 0,
		To:   0,
	}

	convRng, err := ConvertRange(chain, rng)

	assert.NoError(t, err)
	assert.Equal(t, expectedZeroRange, convRng)
}

func testConvertRangeWithFromGreaterThanGenesis(t *testing.T, chain *chain.Chain) {
	genesis, err := chain.GetBlockHeader(0)
	if err != nil {
		t.Fatal(err)
	}
	rng := &Range{
		Unit: TimeRangeType,
		From: genesis.Timestamp() + 1_000,
		To:   genesis.Timestamp() + 10_000,
	}
	expectedEmptyRange := &logdb.Range{
		From: math.MaxUint32,
		To:   math.MaxUint32,
	}

	convRng, err := ConvertRange(chain, rng)

	assert.NoError(t, err)
	assert.Equal(t, expectedEmptyRange, convRng)
}

func testMultipleConvertEventFilters(t *testing.T, chain *chain.Chain) {
	addr := thor.MustParseAddress("0x1234567890123456789012345678901234567890")
	t0 := thor.MustParseBytes32("0x1234567890123456789012345678901234567890123456789012345678901234")
	multipleFilters := &EventFilter{
		CriteriaSet: []*EventCriteria{
			{
				Address: &addr,
				TopicSet: TopicSet{
					Topic0: &t0,
					Topic1: &t0,
					Topic2: &t0,
					Topic3: &t0,
					Topic4: &t0,
				},
			},
		},
		Range:   nil,
		Options: nil,
		Order:   logdb.DESC,
	}

	convRng, err := convertEventFilter(chain, multipleFilters)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(convRng.CriteriaSet))
	assert.Equal(t, addr, *convRng.CriteriaSet[0].Address)
	for i := 0; i < 5; i++ {
		assert.Equal(t, t0, *convRng.CriteriaSet[0].Topics[i])
	}
}

func testMultipleConvertEvent(t *testing.T) {
	var topics [5]*thor.Bytes32
	for i := 0; i < 5; i++ {
		var temp thor.Bytes32
		topics[i] = &temp
	}

	event := &logdb.Event{
		BlockNumber: 1,
		Index:       1,
		BlockID:     thor.MustParseBytes32("0x1234567890123456789012345678901234567890123456789012345678901234"),
		BlockTime:   1,
		TxID:        thor.MustParseBytes32("0x1234567890123456789012345678901234567890123456789012345678901234"),
		TxOrigin:    thor.MustParseAddress("0x1234567890123456789012345678901234567890"),
		ClauseIndex: 1,
		Address:     thor.MustParseAddress("0x1234567890123456789012345678901234567890"),
		Topics:      topics,
		Data:        []byte{0x00, 0x01, 0x02},
	}

	filteredEvents := convertEvent(event)

	assert.Equal(t, 5, len(filteredEvents.Topics))
	for i := 0; i < 5; i++ {
		assert.NotEmpty(t, thor.Bytes32{}, *filteredEvents.Topics[i])
	}
}

// Init functions
func initChain(t *testing.T) *chain.Chain {
	muxDb := muxdb.NewMem()
	stater := state.NewStater(muxDb)
	gene := genesis.NewDevnet()

	b, _, _, err := gene.Build(stater)
	if err != nil {
		t.Fatal(err)
	}

	repo, err := chain.NewRepository(muxDb, b)
	if err != nil {
		t.Fatal(err)
	}

	return repo.NewBestChain()
}
