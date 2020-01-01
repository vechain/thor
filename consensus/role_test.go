package consensus

import (
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

func newBlock(parent *block.Block, score uint64) *block.Block {
	b := new(block.Builder).ParentID(parent.Header().ID()).TotalScore(parent.Header().TotalScore() + score).Build()
	sig, _ := crypto.Sign(b.Header().SigningHash().Bytes(), privateKey)
	return b.WithSignature(sig)
}
