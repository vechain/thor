package consensus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func simpleConsensus() (*Consensus, error) {
	kv, _ := lvldb.NewMem()
	g := genesis.NewDevnet()
	s := state.NewCreator(kv)
	b0, _, err := g.Build(s)
	if err != nil {
		return nil, err
	}

	chain, err := chain.New(kv, b0)
	if err != nil {
		return nil, err
	}

	// forkConfig := thor.ForkConfig{
	// 	VIP191:    0, // enable vip191
	// 	ETH_CONST: 0, // enable latest evm
	// 	BLOCKLIST: math.MaxUint32,
	// }

	cons := New(chain, s, thor.ForkConfig{})
	// packer := packer.New(chain, s,
	// 	genesis.DevAccounts()[0].Address,
	// 	&genesis.DevAccounts()[0].Address,
	// 	forkConfig)

	return cons, nil
}

// // n : number of rounds between the parent and current block
// func newBlock(packer *packer.Packer, parent *block.Block, n uint64, privateKey *ecdsa.PrivateKey) (*block.Block, *state.Stage, error) {
// 	now := parent.Header().Timestamp() + thor.BlockInterval*uint64(n)
// 	// s := parent.Header().TotalScore() + 1
// 	// b := new(block.Builder).ParentID(parent.Header().ID()).Timestamp(t).TotalScore(s).Build()
// 	// sig, _ := crypto.Sign(b.Header().SigningHash().Bytes(), privateKey)
// 	// return b.WithSignature(sig)

// 	// flow, err := packer.Mock(parent.Header(), t, thor.InitialGasLimit)
// 	flow, err := packer.Schedule(parent.Header(), now)
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	if _, _, err := flow.PackTxSetAndBlockSummary(genesis.DevAccounts()[0].PrivateKey); err != nil {
// 		return nil, nil, err
// 	}

// 	// flow.IncTotalScore(1)
// 	b, stage, _, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey)
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	return b, stage, nil
// }

func addEmptyBlocks(tc *testConsensus, nRound uint32, roundSkipped map[uint32]interface{}) map[uint32]*block.Block {
	blks := make(map[uint32]*block.Block)

	for r := uint32(1); r <= nRound; r++ {
		if _, ok := roundSkipped[r]; ok {
			continue
		}

		tc.newBlock(r, nil)
		tc.commitNewBlock()
	}

	return blks
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
	tc := newTestConsensus(t)

	var (
		// currBlock, prevBlock *block.Block
		// err    error
		beacon thor.Bytes32
		epoch  uint32
	)

	// skip the entire second epoch
	roundSkipped := prepareSkippedRoundInfo()
	nRound := uint32(thor.EpochInterval) * 3
	blocks := addEmptyBlocks(tc, nRound, roundSkipped)

	// Test beacon for epoch 1
	epoch = 1
	beacon, err := tc.con.beacon(epoch)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, beacon.Bytes(), getBeaconFromHeader(tc.genesisBlock.Header()).Bytes())

	// Test beacon for epoch 2 => beacon from the last block of epoch 1
	epoch = 2
	beacon, err = tc.con.beacon(epoch)
	if err != nil {
		t.Fatal(err)
	}
	block := blocks[uint32(thor.EpochInterval)]
	assert.Equal(t, beacon.Bytes(), getBeaconFromHeader(block.Header()).Bytes())

	// Test beacon for epoch 3 => beacon from the last block of epoch 1
	epoch = 3
	beacon, err = tc.con.beacon(epoch)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, beacon.Bytes(), getBeaconFromHeader(block.Header()).Bytes())
}
