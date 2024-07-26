// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package events_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
)

func TestEventsTypes(t *testing.T) {
	c := initChain(t)
	for name, tt := range map[string]func(*testing.T, *chain.Chain){
		"testConvertRangeWithBlockRangeType":               testConvertRangeWithBlockRangeType,
		"testConvertRangeWithTimeRangeTypeLessThenGenesis": testConvertRangeWithTimeRangeTypeLessThenGenesis,
		"testConvertRangeWithTimeRangeType":                testConvertRangeWithTimeRangeType,
		"testConvertRangeWithFromGreaterThanGenesis":       testConvertRangeWithFromGreaterThanGenesis,
	} {
		t.Run(name, func(t *testing.T) {
			tt(t, c)
		})
	}
}

func testConvertRangeWithBlockRangeType(t *testing.T, chain *chain.Chain) {
	rng := &events.Range{
		Unit: events.BlockRangeType,
		From: 1,
		To:   2,
	}

	convertedRng, err := events.ConvertRange(chain, rng)

	assert.NoError(t, err)
	assert.Equal(t, uint32(rng.From), convertedRng.From)
	assert.Equal(t, uint32(rng.To), convertedRng.To)
}

func testConvertRangeWithTimeRangeTypeLessThenGenesis(t *testing.T, chain *chain.Chain) {
	rng := &events.Range{
		Unit: events.TimeRangeType,
		From: 1,
		To:   2,
	}
	expectedEmptyRange := &logdb.Range{
		From: math.MaxUint32,
		To:   math.MaxUint32,
	}

	convRng, err := events.ConvertRange(chain, rng)

	assert.NoError(t, err)
	assert.Equal(t, expectedEmptyRange, convRng)
}

func testConvertRangeWithTimeRangeType(t *testing.T, chain *chain.Chain) {
	genesis, err := chain.GetBlockHeader(0)
	if err != nil {
		t.Fatal(err)
	}
	rng := &events.Range{
		Unit: events.TimeRangeType,
		From: 1,
		To:   genesis.Timestamp(),
	}
	expectedZeroRange := &logdb.Range{
		From: 0,
		To:   0,
	}

	convRng, err := events.ConvertRange(chain, rng)

	assert.NoError(t, err)
	assert.Equal(t, expectedZeroRange, convRng)
}

func testConvertRangeWithFromGreaterThanGenesis(t *testing.T, chain *chain.Chain) {
	genesis, err := chain.GetBlockHeader(0)
	if err != nil {
		t.Fatal(err)
	}
	rng := &events.Range{
		Unit: events.TimeRangeType,
		From: genesis.Timestamp() + 1_000,
		To:   genesis.Timestamp() + 10_000,
	}
	expectedEmptyRange := &logdb.Range{
		From: math.MaxUint32,
		To:   math.MaxUint32,
	}

	convRng, err := events.ConvertRange(chain, rng)

	assert.NoError(t, err)
	assert.Equal(t, expectedEmptyRange, convRng)
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
