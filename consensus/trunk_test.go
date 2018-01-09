package consensus_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/dsa"
	"github.com/vechain/thor/genesis/builder"
	"github.com/vechain/thor/genesis/contracts"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
)

func TestPredicateTrunk(t *testing.T) {
	db, _ := lvldb.NewMem()
	state, _ := state.New(cry.Hash{}, db)
	defer db.Close()

	signer := acc.Address(crypto.PubkeyToAddress(key.PublicKey))

	if genesisBlk, err := buildGenesis(state, signer); err != nil {
		t.Fatal(err)
	} else {
		blk := new(block.Builder).
			ParentHash(genesisBlk.Hash()).
			Beneficiary(signer).
			Timestamp(1234567890 + 10).
			Build()
		sig, _ := dsa.Sign(blk.Header().HashForSigning(), crypto.FromECDSA(key))
		blk = blk.WithSignature(sig)
		t.Log(consensus.PredicateTrunk(state, blk.Header(), genesisBlk.Header()))
	}
}

func buildGenesis(state *state.State, signer acc.Address) (*block.Block, error) {
	return new(builder.Builder).
		Timestamp(1234567890).
		GasLimit(big.NewInt(10*1000*1000)).
		Alloc(
			contracts.Authority.Address,
			new(big.Int),
			contracts.Authority.RuntimeBytecodes(),
		).
		Call(
			contracts.Authority.Address,
			func() []byte {
				data, err := contracts.Authority.ABI.Pack(
					"initialize",
					acc.BytesToAddress([]byte("test")),
					[]acc.Address{signer})
				if err != nil {
					panic(errors.Wrap(err, "build genesis"))
				}
				return data
			}(),
		).
		Build(state, acc.BytesToAddress([]byte("god")))
}
