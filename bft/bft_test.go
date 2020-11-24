package bft

import (
	"crypto/ecdsa"
	"math/rand"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
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
			state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
			for _, node := range nodes {
				ok, err := builtin.Authority.Native(state).Add(pubToAddr(
					node.PublicKey), thor.Address{}, thor.Bytes32{},
				)
				if !ok {
					panic("failed to add consensus node")
				}
				if err != nil {
					panic(err)
				}
			}
			return nil
		})

	return
}

func newTestRepo() *chain.Repository {
	db := muxdb.NewMem()
	// g := genesis.NewDevnet()
	g := newTestGenesisBuilder()
	b0, _, _, _ := g.Build(state.NewStater(db))

	repo, err := chain.NewRepository(db, b0)
	if err != nil {
		panic(err)
	}
	return repo
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

	msg := block.NewProposal(
		h.ID(),
		emptyRootHash,
		h.GasLimit(),
		timestamp,
	).AsMessage(thor.Address(crypto.PubkeyToAddress(nodes[proposer].PublicKey)))

	bss := block.ComplexSignatures(nil)
	for _, backer := range backers {
		proof := make([]byte, 81)
		rand.Read(proof)
		sig, err := crypto.Sign(thor.Blake2b(msg, proof).Bytes(), nodes[backer])
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
