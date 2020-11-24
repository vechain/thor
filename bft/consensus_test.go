package bft

import (
	"crypto/ecdsa"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

func TestNormalSituation(t *testing.T) {
	var (
		head         *block.Block
		branch       *chain.Chain
		v            *view
		err          error
		maxBlockNum  uint32 = 20
		expNumBacker        = 40
		N                   = int(thor.MaxBlockProposers)

		leader  *ecdsa.PrivateKey
		backers []*ecdsa.PrivateKey

		cons      *Consensus
		nv        thor.Bytes32
		prevState [5]thor.Bytes32
	)

	repo, nodes := newTestRepo()
	head = repo.GenesisBlock()

	cons = NewConsensus(repo, head.Header().ID(), pubToAddr(nodes[rand.Intn(N)].PublicKey))

	branch = repo.NewChain(head.Header().ID())
	v, err = newView(branch, block.Number(head.Header().NV()))
	assert.Nil(t, err)

	for {
		// Randomly pick the leader and backers
		leader = nodes[rand.Intn(N)]
		backers = nil
		for i := 0; i < N; i++ {
			if rand.Intn(N) < expNumBacker {
				backers = append(backers, nodes[i])
			}
		}

		// Use the new block's ID as the NV value if the block is the first block of its view
		nv = cons.state[NV]
		if v.hasQCForNV() || head.Header().Number() == 0 {
			nv = GenNVforFirstBlock(head.Header().Number() + 1)
		}

		head = newBlock(
			leader,
			backers,
			head.Header().ID(),
			head.Header().Timestamp()+thor.BlockInterval,
			0,
			[4]thor.Bytes32{
				nv,
				cons.state[PP],
				cons.state[PC],
				cons.state[CM],
			},
		)

		repo.AddBlock(head, nil)

		prevState = cons.Get()
		cons.Update(head)

		// In case of a block starting a new view, check the NV value change
		if v.hasQCForNV() {
			expected := prevState
			expected[NV] = head.Header().ID()
			assert.Equal(t, expected, cons.Get())
		}

		branch = repo.NewChain(head.Header().ID())
		v, err = newView(branch, block.Number(head.Header().NV()))
		assert.Nil(t, err)

		// In case of a view with QC, PP <- NV, PC <- PP, FN <- new PC
		if v.hasQCForNV() {
			expected := prevState
			expected[NV] = prevState[NV]
			expected[PP] = prevState[NV]
			expected[PC] = prevState[PP]
			expected[CM] = prevState[PC]
			if !expected[CM].IsZero() {
				expected[FN] = expected[CM]
			}
			assert.Equal(t, expected, cons.Get())
		}

		printFinalityState(head.Header().Number(), v, cons)

		if head.Header().Number() >= maxBlockNum {
			break
		}
	}
}

func TestCommitFromBlockMessages(t *testing.T) {
	repo, nodes := newTestRepo()
	gen := repo.BestBlock()
	cons := NewConsensus(repo, gen.Header().ID(), pubToAddr(nodes[0].PublicKey))

	leader := nodes[0]
	backers := nodes[:QC]

	var blocks []*block.Block
	blocks = append(blocks, gen)

	emptyBytes32 := thor.Bytes32{}
	for i := 1; i <= 3; i++ {
		fv := [4]thor.Bytes32{GenNVforFirstBlock(uint32(i))}
		if i == 3 {
			fv = [4]thor.Bytes32{
				GenNVforFirstBlock(uint32(i)),
				emptyBytes32,
				emptyBytes32,
				blocks[2].Header().ID(),
			}
		}

		b := newBlock(
			leader,
			backers,
			blocks[i-1].Header().ID(),
			blocks[i-1].Header().Timestamp()+thor.BlockInterval,
			0,
			fv,
		)
		assert.Nil(t, repo.AddBlock(b, nil))
		blocks = append(blocks, b)
	}

	// Observed CM not newer than the locally committed
	cons.state[CM] = blocks[3].Header().ID()
	assert.Nil(t, cons.Update(blocks[3]))
	assert.Equal(t, blocks[3].Header().ID(), cons.state[CM])

	// Observed CM newer than the locally committed
	cons.state = [5]thor.Bytes32{}
	cons.state[CM] = blocks[1].Header().ID()
	assert.Nil(t, cons.Update(blocks[3]))
	assert.Equal(t, blocks[2].Header().ID(), cons.state[CM])
}

func printFinalityState(n uint32, v *view, cons *Consensus) {
	fmt.Printf("Blk%d, sigSize = %d, finalVec = [ %d, %d, %d, %d, %d ]\n",
		n,
		len(v.nv),
		block.Number(cons.state[NV]),
		block.Number(cons.state[PP]),
		block.Number(cons.state[PC]),
		block.Number(cons.state[CM]),
		block.Number(cons.state[FN]),
	)
}
