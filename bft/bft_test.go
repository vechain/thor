package bft

import (
	"bytes"
	"crypto/ecdsa"
	"math/rand"
	"sort"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/go-ecvrf"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

var (
	nodes   []*ecdsa.PrivateKey
	inds    []int
	emptyID = thor.Bytes32{}
)

func init() {
	for i := 0; i < int(thor.MaxBlockProposers); i++ {
		pk, _ := crypto.GenerateKey()
		nodes = append(nodes, pk)
		inds = append(inds, i)
	}
}

func M(args ...interface{}) []interface{} {
	return args
}

func randBytes32() (b thor.Bytes32) {
	rand.Read(b[:])
	return
}

func countIntArray(a []int) (c map[int]int) {
	c = make(map[int]int)

	for _, e := range a {
		c[e] = c[e] + 1
	}

	return
}

func pubToAddr(pub ecdsa.PublicKey) thor.Address {
	return thor.Address(crypto.PubkeyToAddress(pub))
}

func nodeAddress(i int) thor.Address {
	return pubToAddr(nodes[i].PublicKey)
}

func newTestGenesisBuilder() (builder *genesis.Builder) {
	builder = new(genesis.Builder).
		Timestamp(0).
		GasLimit(thor.InitialGasLimit).
		State(func(state *state.State) error { // add master nodes
			if err := state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes()); err != nil {
				panic(err)
			}
			if err := state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes()); err != nil {
				panic(err)
			}

			if err := builtin.Params.Native(state).Set(thor.KeyProposerEndorsement, thor.InitialProposerEndorsement); err != nil {
				panic(err)
			}

			for i := range nodes {
				ok, err := builtin.Authority.Native(state).Add(nodeAddress(i), nodeAddress(i), thor.Bytes32{})
				if !ok {
					panic("failed to add consensus node")
				}
				if err != nil {
					panic(err)
				}

				if err := state.SetBalance(nodeAddress(i), thor.InitialProposerEndorsement); err != nil {
					return err
				}
			}
			return nil
		})

	return
}

func newTestRepo() (repo *chain.Repository, db *muxdb.MuxDB) {
	db = muxdb.NewMem()
	g := newTestGenesisBuilder()
	b0, _, _, _ := g.Build(state.NewStater(db))
	repo, _ = chain.NewRepository(db, b0)
	return
}

func newBlock(
	proposer int,
	backers []int,
	parent *block.Block,
	numInternvalApart uint64,
	fv [4]thor.Bytes32,
) (blk *block.Block) {
	h := parent.Header()
	timestamp := h.Timestamp() + numInternvalApart*thor.BlockInterval

	hash := block.NewProposal(
		h.ID(),
		emptyRootHash,
		h.GasLimit(),
		timestamp,
	).Hash()

	bss := block.ComplexSignatures(nil)
	for _, backer := range backers {
		proof := make([]byte, 81)
		rand.Read(proof)
		sig, err := crypto.Sign(hash.Bytes(), nodes[backer])
		if err != nil {
			panic(err)
		}
		bs, err := block.NewComplexSignature(proof, sig)
		if err != nil {
			panic(err)
		}
		bss = append(bss, bs)
	}

	builder := new(block.Builder).
		ParentID(h.ID()).
		Timestamp(timestamp).
		GasLimit(h.GasLimit()).
		BackerSignatures(bss, 0, 0).
		FinalityVector(fv)

	blk = builder.Build()
	sig, err := crypto.Sign(blk.Header().SigningHash().Bytes(), nodes[proposer])
	if err != nil {
		panic(err)
	}
	blk = blk.WithSignature(sig)

	return
}

func newBlock1(
	proposer int,
	backers []int,
	parent *block.Block,
	blockTime uint64,
	totalScore uint64,
	seed []byte,
	stateRoot thor.Bytes32,
	fv [4]thor.Bytes32,
) (blk *block.Block, err error) {
	h := parent.Header()

	hash := block.NewProposal(
		h.ID(),
		emptyRootHash,
		h.GasLimit(),
		blockTime,
	).Hash()

	bss := block.ComplexSignatures(nil)
	alpha := append([]byte(nil), seed...)
	alpha = append(alpha, parent.Header().ID().Bytes()[:4]...)

	type betaInd struct {
		beta []byte
		i    int
	}
	var betaInds []betaInd

	for i, backer := range backers {
		sig, err := crypto.Sign(hash.Bytes(), nodes[backer])
		if err != nil {
			return nil, err
		}

		beta, proof, err := ecvrf.NewSecp256k1Sha256Tai().Prove(nodes[backer], alpha)
		if err != nil {
			return nil, err
		}

		bs, err := block.NewComplexSignature(proof, sig)
		if err != nil {
			return nil, err
		}

		bss = append(bss, bs)
		betaInds = append(betaInds, betaInd{beta, i})
	}

	var bss1 block.ComplexSignatures
	if len(bss) > 0 {
		sort.Slice(betaInds, func(i, j int) bool {
			return bytes.Compare(betaInds[i].beta, betaInds[j].beta) < 0
		})

		for _, ind := range betaInds {
			bss1 = append(bss1, bss[ind.i])
		}
	}

	builder := new(block.Builder).
		ParentID(h.ID()).
		Timestamp(blockTime).
		GasLimit(h.GasLimit()).
		BackerSignatures(bss1, 0, 0).
		FinalityVector(fv).
		TotalScore(totalScore).
		ReceiptsRoot(tx.Receipts(nil).RootHash()).
		StateRoot(stateRoot)

	blk = builder.Build()
	sig, err := crypto.Sign(blk.Header().SigningHash().Bytes(), nodes[proposer])
	if err != nil {
		return nil, err
	}
	blk = blk.WithSignature(sig)

	return
}
