package consensus

import (
	"bytes"
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

var privateKey, _ = crypto.GenerateKey()

// n : number of rounds between the parent and current block
func newBlock(parent *block.Block, n uint64) *block.Block {
	t := parent.Header().Timestamp() + thor.BlockInterval*n
	b := new(block.Builder).ParentID(parent.Header().ID()).Timestamp(t).Build()
	sig, _ := crypto.Sign(b.Header().SigningHash().Bytes(), privateKey)
	return b.WithSignature(sig)
}

func TestBeacon(t *testing.T) {
	cons := initConsensus()
	gen := cons.chain.GenesisBlock()

	var (
		currBlock, prevBlock, lastBlock *block.Block
		err                             error
		beacon                          thor.Bytes32
	)

	prevBlock = gen
	// To set thor.EpochInveral = 10 for testing purpose
	for i := uint64(0); i < thor.EpochInterval*2; i++ {
		// Skip the block at the last round of the first epoch
		if i == thor.EpochInterval {
			lastBlock = prevBlock
			continue
		}

		currBlock = newBlock(prevBlock, 1)
		_, err = cons.chain.AddBlock(currBlock, nil)
		if err != nil {
			panic(err)
		}
		prevBlock = currBlock
	}

	// Test beacon for the first epoch
	beacon, err = cons.Beacon(uint32(1))
	if err != nil {
		panic(err)
	}
	if bytes.Compare(beacon.Bytes(), gen.Header().ID().Bytes()) != 0 {
		t.Errorf("Test failed")
	}

	// Test beacon for the second epoch
	beacon, err = cons.Beacon(uint32(thor.EpochInterval) + 1)
	if err != nil {
		panic(err)
	}
	if bytes.Compare(beacon.Bytes(), lastBlock.Header().ID().Bytes()) != 0 {
		t.Errorf("Test failed")
	}
}
