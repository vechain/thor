package consensus

import (
	"bytes"
	"crypto/ecdsa"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func initConsensus() *Consensus {
	kv, _ := lvldb.NewMem()
	g := genesis.NewDevnet()
	s := state.NewCreator(kv)
	b0, _, _ := g.Build(s)

	chain, err := chain.New(kv, b0)
	if err != nil {
		panic(err)
	}

	return New(chain, s, thor.ForkConfig{})
}

// n : number of rounds between the parent and current block
func newBlock(parent *block.Block, n uint64, privateKey *ecdsa.PrivateKey) *block.Block {
	t := parent.Header().Timestamp() + thor.BlockInterval*uint64(n)
	s := parent.Header().TotalScore() + 1
	b := new(block.Builder).ParentID(parent.Header().ID()).Timestamp(t).TotalScore(s).Build()
	sig, _ := crypto.Sign(b.Header().SigningHash().Bytes(), privateKey)
	return b.WithSignature(sig)
}

func addEmptyBlocks(
	chain *chain.Chain,
	privateKey *ecdsa.PrivateKey,
	nRound uint32,
	roundSkipped map[uint32]interface{},
) (map[uint32]*block.Block, error) {
	var prev, curr *block.Block

	blks := make(map[uint32]*block.Block)

	prev = chain.GenesisBlock()
	n := uint64(1)

	for r := uint32(1); r <= nRound; r++ {
		if _, ok := roundSkipped[r]; ok {
			n = n + 1
			continue
		}

		curr = newBlock(prev, n, privateKey)
		_, err := chain.AddBlock(curr, nil)
		if err != nil {
			return nil, err
		}

		blks[r] = curr
		n = 1
		prev = curr
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
	privateKey, _ := crypto.GenerateKey()

	cons := initConsensus()
	gen := cons.chain.GenesisBlock()

	var (
		// currBlock, prevBlock *block.Block
		err    error
		beacon thor.Bytes32
		epoch  uint32
	)

	// var roundSkipped = map[uint32]interface{}{uint32(thor.EpochInterval): struct{}{}}
	roundSkipped := prepareSkippedRoundInfo()
	nRound := uint32(thor.EpochInterval) * 3
	blocks, err := addEmptyBlocks(cons.chain, privateKey, nRound, roundSkipped)
	if err != nil {
		t.Fatal(err)
	}

	// Test beacon for epoch 1
	epoch = 1
	beacon, err = cons.beacon(epoch)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(beacon.Bytes(), getBeaconFromHeader(gen.Header()).Bytes()) != 0 {
		t.Errorf("Test1 failed")
	}

	// Test beacon for epoch 2 => beacon from the last block of epoch 1
	epoch = 2
	beacon, err = cons.beacon(epoch)
	if err != nil {
		t.Fatal(err)
	}
	block := blocks[uint32(thor.EpochInterval)]
	if bytes.Compare(beacon.Bytes(), getBeaconFromHeader(block.Header()).Bytes()) != 0 {
		t.Errorf("Test2 failed")
	}

	// Test beacon for epoch 3 => beacon from the last block of epoch 1
	epoch = 3
	beacon, err = cons.beacon(epoch)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(beacon.Bytes(), getBeaconFromHeader(block.Header()).Bytes()) != 0 {
		t.Errorf("Test3 failed")
	}
}
