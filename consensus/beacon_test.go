package consensus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

func addEmptyBlocks(tc *TempChain, nRound uint32, roundSkipped map[uint32]interface{}) (map[uint32]*block.Block, error) {
	blks := make(map[uint32]*block.Block)

	for r := uint32(1); r <= nRound; r++ {
		if _, ok := roundSkipped[r]; ok {
			continue
		}

		if err := tc.NewBlock(r, nil); err != nil {
			return blks, err
		}
		if err := tc.CommitNewBlock(); err != nil {
			return blks, err
		}

		blks[r] = tc.Original
	}

	return blks, nil
}

func prepareSkippedRoundInfo() map[uint32]interface{} {
	// skip the entire second epoch
	skip := make(map[uint32]interface{})

	for i := uint32(thor.EpochInterval*1 + 1); i <= uint32(thor.EpochInterval*2); i++ {
		skip[i] = struct{}{}
	}

	return skip
}

func TestBeacon(t *testing.T) {
	var (
		tc     *TempChain
		beacon thor.Bytes32
		epoch  uint32
		err    error
	)

	tc, err = NewTempChain(3, thor.ForkConfig{})
	if err != nil {
		t.Fatal(err)
	}

	// skip the entire second epoch
	roundSkipped := prepareSkippedRoundInfo()
	nRound := uint32(thor.EpochInterval) * 3
	blocks, err := addEmptyBlocks(tc, nRound, roundSkipped)
	if err != nil {
		t.Fatal(err)
	}

	// Test beacon for epoch 1
	epoch = 1
	beacon, err = tc.Con.beacon(epoch)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, beacon.Bytes(), compBeacon(tc.GenesisBlock.Header()).Bytes())

	// Test beacon for epoch 2 => beacon from the last block of epoch 1
	epoch = 2
	beacon, err = tc.Con.beacon(epoch)
	if err != nil {
		t.Fatal(err)
	}
	block := blocks[uint32(thor.EpochInterval)]
	assert.Equal(t, beacon.Bytes(), compBeacon(block.Header()).Bytes())

	// Test beacon for epoch 3 => beacon from the last block of epoch 1
	epoch = 3
	beacon, err = tc.Con.beacon(epoch)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, beacon.Bytes(), compBeacon(block.Header()).Bytes())
}
