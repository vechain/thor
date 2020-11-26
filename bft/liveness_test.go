package bft

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/thor"
)

var (
	repos     []*chain.Repository
	cons      []*Consensus
	seeder    *poa.Seeder
	proposers []poa.Proposer

	a1, a2, a3, a4, a5, a6 *block.Block
	b1, b2, b3, b4, b5, b6 *block.Block
	c1, c2, c3, c4, c5, c6 *block.Block
)

func randQC() (qc []int) {
	r := make(map[int]interface{})

	for {
		if len(r) >= QC {
			break
		}

		i := rand.Intn(nNode)
		if _, ok := r[i]; !ok {
			r[i] = struct{}{}
		}
	}

	for k := range r {
		qc = append(qc, k)
	}

	return
}

func getProposer(parent *block.Block, nInterval int) int {
	blockTime := parent.Header().Timestamp() + uint64(nInterval)*thor.BlockInterval
	u := 0

	seed, err := seeder.Generate(parent.Header().ID())
	if err != nil {
		panic(err)
	}

	sche, err := poa.NewSchedulerV2(nodeAddress(u), proposers, parent, seed.Bytes())
	if err != nil {
		panic(err)
	}

	for i := 0; i < nNode; i++ {
		if sche.IsScheduled(blockTime, nodeAddress(i)) {
			return i
		}
	}

	panic("No valid proposer")
}

func initNodesStatus(t *testing.T) {
	t1 := int(QC / 3)
	t2 := int(2 * QC / 3)

	// Init repository and consensus for nodes
	for i := 0; i < nNode; i++ {
		repos = append(repos, newTestRepo())
		cons = append(cons, NewConsensus(repos[i], repos[i].GenesisBlock().Header().ID(), nodeAddress(i)))
		proposers = append(proposers, poa.Proposer{Address: nodeAddress(i), Active: true})
	}
	seeder = poa.NewSeeder(repos[0])

	gen := repos[0].GenesisBlock()
	backers := randQC()
	a1 = newBlock(rand.Intn(nNode), backers[:t1], gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 = newBlock(rand.Intn(nNode), backers[t1:t2], a1, 1, [4]thor.Bytes32{a1.Header().ID()})
	a3 = newBlock(rand.Intn(nNode), backers[t2:], a2, 1, [4]thor.Bytes32{a1.Header().ID()})
	backers = randQC()
	a4 = newBlock(rand.Intn(nNode), backers[:t1], a3, 1, [4]thor.Bytes32{GenNVForFirstBlock(4), a1.Header().ID()})
	a5 = newBlock(rand.Intn(nNode), backers[t1:t2], a4, 1, [4]thor.Bytes32{a4.Header().ID(), a1.Header().ID()})
	a6 = newBlock(rand.Intn(nNode), backers[t2:], a5, 1, [4]thor.Bytes32{a4.Header().ID(), a1.Header().ID()})

	backers = randQC()
	b1 = newBlock(rand.Intn(nNode), backers[:t1], gen, 2, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	b2 = newBlock(rand.Intn(nNode), backers[t1:t2], b1, 1, [4]thor.Bytes32{b1.Header().ID()})
	b3 = newBlock(rand.Intn(nNode), backers[t2:], b2, 1, [4]thor.Bytes32{b1.Header().ID()})
	backers = randQC()
	b4 = newBlock(rand.Intn(nNode), backers[:t1], b3, 1, [4]thor.Bytes32{GenNVForFirstBlock(4), b1.Header().ID()})
	b5 = newBlock(rand.Intn(nNode), backers[t1:t2], b4, 1, [4]thor.Bytes32{b4.Header().ID(), b1.Header().ID()})
	b6 = newBlock(rand.Intn(nNode), backers[t2:], b5, 1, [4]thor.Bytes32{b4.Header().ID(), b1.Header().ID()})

	backers = randQC()
	c1 = newBlock(rand.Intn(nNode), backers[:t1], gen, 3, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	c2 = newBlock(rand.Intn(nNode), backers[t1:t2], c1, 1, [4]thor.Bytes32{c1.Header().ID()})
	c3 = newBlock(rand.Intn(nNode), backers[t2:], c2, 1, [4]thor.Bytes32{c1.Header().ID()})
	backers = randQC()
	c4 = newBlock(rand.Intn(nNode), backers[:t1], c3, 1, [4]thor.Bytes32{GenNVForFirstBlock(4), c1.Header().ID()})
	c5 = newBlock(rand.Intn(nNode), backers[t1:t2], c4, 1, [4]thor.Bytes32{c4.Header().ID(), c1.Header().ID()})
	c6 = newBlock(rand.Intn(nNode), backers[t2:], c5, 1, [4]thor.Bytes32{c4.Header().ID(), c1.Header().ID()})

	for u := 0; u <= 32; u++ {
		repos[u].AddBlock(a1, nil)
		repos[u].AddBlock(a2, nil)
		repos[u].AddBlock(a3, nil)
		repos[u].AddBlock(a4, nil)
		repos[u].AddBlock(a5, nil)
		repos[u].AddBlock(a6, nil)
		repos[u].AddBlock(b1, nil)
		repos[u].AddBlock(b2, nil)
		repos[u].AddBlock(b3, nil)
		repos[u].AddBlock(b4, nil)
		repos[u].AddBlock(b5, nil)
		repos[u].AddBlock(c1, nil)
		repos[u].AddBlock(c2, nil)
		repos[u].AddBlock(c3, nil)
		repos[u].AddBlock(c4, nil)

		cons[u].repo.SetBestBlockID(a6.Header().ID())
		assert.Nil(t, cons[u].Update(a6))

		assert.Equal(t, a4.Header().ID(), cons[u].state[NV])
		assert.Equal(t, a4.Header().ID(), cons[u].state[PP])
		assert.Equal(t, a1.Header().ID(), cons[u].state[PC])
	}

	for u := 33; u <= 65; u++ {
		repos[u].AddBlock(a1, nil)
		repos[u].AddBlock(a2, nil)
		repos[u].AddBlock(a3, nil)
		repos[u].AddBlock(a4, nil)
		repos[u].AddBlock(a5, nil)
		repos[u].AddBlock(a6, nil)
		repos[u].AddBlock(b1, nil)
		repos[u].AddBlock(b2, nil)
		repos[u].AddBlock(b3, nil)
		repos[u].AddBlock(b4, nil)
		repos[u].AddBlock(b5, nil)
		repos[u].AddBlock(b6, nil)
		repos[u].AddBlock(c1, nil)
		repos[u].AddBlock(c2, nil)
		repos[u].AddBlock(c3, nil)
		repos[u].AddBlock(c4, nil)
		repos[u].AddBlock(c5, nil)

		cons[u].repo.SetBestBlockID(b6.Header().ID())
		assert.Nil(t, cons[u].Update(b6))

		assert.Equal(t, b4.Header().ID(), cons[u].state[NV])
		assert.Equal(t, b4.Header().ID(), cons[u].state[PP])
		assert.Equal(t, b1.Header().ID(), cons[u].state[PC])
	}

	for u := 66; u <= 100; u++ {
		repos[u].AddBlock(a1, nil)
		repos[u].AddBlock(a2, nil)
		repos[u].AddBlock(a3, nil)
		repos[u].AddBlock(a4, nil)
		repos[u].AddBlock(a5, nil)
		repos[u].AddBlock(a6, nil)
		repos[u].AddBlock(b1, nil)
		repos[u].AddBlock(b2, nil)
		repos[u].AddBlock(b3, nil)
		repos[u].AddBlock(b4, nil)
		repos[u].AddBlock(b5, nil)
		repos[u].AddBlock(b6, nil)
		repos[u].AddBlock(c1, nil)
		repos[u].AddBlock(c2, nil)
		repos[u].AddBlock(c3, nil)
		repos[u].AddBlock(c4, nil)
		repos[u].AddBlock(c5, nil)
		repos[u].AddBlock(c6, nil)

		cons[u].repo.SetBestBlockID(c6.Header().ID())
		assert.Nil(t, cons[u].Update(c6))

		assert.Equal(t, c4.Header().ID(), cons[u].state[NV])
		assert.Equal(t, c4.Header().ID(), cons[u].state[PP])
		assert.Equal(t, c1.Header().ID(), cons[u].state[PC])
	}
}

func TestLiveness1(t *testing.T) {
	initNodesStatus(t)

	for u := 0; u <= 32; u++ {
		repos[u].AddBlock(b6, nil)
		repos[u].AddBlock(c5, nil)
		repos[u].AddBlock(c6, nil)

		repos[u].SetBestBlockID(c6.Header().ID())
		assert.Nil(t, cons[u].Update(c6))

		assert.Equal(t, c4.Header().ID(), cons[u].state[NV])
		assert.Equal(t, c4.Header().ID(), cons[u].state[PP])
		assert.Equal(t, c1.Header().ID(), cons[u].state[PC])
	}
}
