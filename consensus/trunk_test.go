package consensus_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func TestPredicateTrunk(t *testing.T) {
	db, _ := lvldb.NewMem()
	state, _ := state.New(thor.Hash{}, db)
	defer db.Close()

	signer := thor.Address(crypto.PubkeyToAddress(key.PublicKey))

	if genesisBlk, err := buildGenesis(state, signer); err != nil {
		t.Fatal(err)
	} else {
		blk := new(block.Builder).
			ParentHash(genesisBlk.Hash()).
			Beneficiary(signer).
			Timestamp(1234567890 + 10).
			Build()
		sig, _ := cry.NewSigning(thor.Hash{}).Sign(blk.Header(), crypto.FromECDSA(key))
		blk = blk.WithSignature(sig)
		//t.Log(consensus.PredicateTrunk(state, blk.Header(), genesisBlk.Header()))
	}
}

func buildGenesis(state *state.State, signer thor.Address) (*block.Block, error) {
	return new(genesis.Builder).
		Timestamp(1234567890).
		GasLimit(10*1000*1000).
		Alloc(
			contracts.Authority.Address,
			new(big.Int),
			contracts.Authority.RuntimeBytecodes(),
		).
		Call(
			contracts.Authority.Address,
			contracts.Authority.PackInitialize(thor.BytesToAddress([]byte("test")))).
		Call(contracts.Authority.Address,
			contracts.Authority.PackPreset(signer, "p1"),
		).
		Build(state)
}
