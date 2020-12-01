package bft

import (
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

		leader  int
		backers []int

		cons      *Consensus
		nv        thor.Bytes32
		prevState []thor.Bytes32
	)

	// printFinalityState := func(n uint32) {
	// 	fmt.Printf("Blk%d, sigSize = %d, finalVec = [ %d, %d, %d, %d]\n",
	// 		n,
	// 		len(v.nv),
	// 		block.Number(cons.state[NV]),
	// 		block.Number(cons.state[PP]),
	// 		block.Number(cons.state[PC]),
	// 		block.Number(cons.state[CM]),
	// 	)
	// }

	repo, _ := newTestRepo()
	head = repo.GenesisBlock()

	// node := pubToAddr(nodes[rand.Intn(N)].PublicKey)
	node := nodeAddress(rand.Intn(N))
	cons = NewConsensus(repo, head.Header().ID(), node)

	branch = repo.NewChain(head.Header().ID())
	v, err = newView(branch, block.Number(head.Header().NV()))
	assert.Nil(t, err)

	for {
		// Randomly pick the leader and backers
		leader = rand.Intn(N)
		backers = nil
		for i := 0; i < N; i++ {
			if rand.Intn(N) < expNumBacker {
				backers = append(backers, i)
			}
		}

		// Use the new block's ID as the NV value if the block is the first block of its view
		nv = cons.state[NV]
		if v.hasQCForNV() || head.Header().Number() == 0 {
			nv = GenNVForFirstBlock(head.Header().Number() + 1)
		}

		head = newBlock(
			leader, backers, head, 1,
			[4]thor.Bytes32{
				nv,
				cons.state[PP],
				cons.state[PC],
				cons.state[CM],
			},
		)

		if cons.IfUpdateLastSignedPC(head) {
			assert.Nil(t, cons.UpdateLastSignedPC(head))
		}

		repo.AddBlock(head, nil)
		repo.SetBestBlockID(head.Header().ID())

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
			assert.Equal(t, prevState, cons.Get())
		}

		// printFinalityState(head.Header().Number())

		if head.Header().Number() >= maxBlockNum {
			break
		}
	}
}

func Test1b(t *testing.T) {
	/**
	gen -- a1 -- a2
	  		\
	   		 -------- b1
	*/
	repo, _ := newTestRepo()
	gen := repo.BestBlock()
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(0))

	a1 := newBlock(0, backers(), gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(0, backers(), a1, 1, [4]thor.Bytes32{a1.Header().ID()})
	b1 := newBlock(0, backers(), a1, 2, [4]thor.Bytes32{GenNVForFirstBlock(2)})

	repo.AddBlock(a1, nil)
	repo.SetBestBlockID(a2.Header().ID())
	assert.Nil(t, cons.Update(a1))
	repo.AddBlock(a2, nil)
	repo.SetBestBlockID(a2.Header().ID())
	assert.Nil(t, cons.Update(a2))
	assert.Equal(t, cons.state[NV], a1.Header().ID())

	repo.AddBlock(b1, nil)
	repo.SetBestBlockID(b1.Header().ID())
	assert.Nil(t, cons.Update(b1))
	assert.Equal(t, cons.state[NV], b1.Header().ID())
}

func Test1c(t *testing.T) {
	/**
	gen -- a1 -----a2
	    	\
	   		 b1 ------ b2
	*/
	repo, _ := newTestRepo()
	gen := repo.BestBlock()
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(0))

	a1 := newBlock(0, backers(), gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(0, backers(), a1, 1, [4]thor.Bytes32{a1.Header().ID()})
	b1 := newBlock(0, backers(), a1, 2, [4]thor.Bytes32{GenNVForFirstBlock(2)})
	b2 := newBlock(0, backers(), b1, 2, [4]thor.Bytes32{b1.Header().ID()})

	repo.AddBlock(a1, nil)
	repo.SetBestBlockID(a2.Header().ID())
	assert.Nil(t, cons.Update(a1))
	repo.AddBlock(a2, nil)
	repo.SetBestBlockID(a2.Header().ID())
	assert.Nil(t, cons.Update(a2))
	assert.Equal(t, cons.state[NV], a1.Header().ID())

	repo.AddBlock(b1, nil)
	assert.Nil(t, cons.Update(b1))
	assert.Equal(t, cons.state[NV], a1.Header().ID())

	repo.AddBlock(b2, nil)
	repo.SetBestBlockID(b2.Header().ID())
	assert.Nil(t, cons.Update(b2))
	assert.Equal(t, cons.state[NV], b1.Header().ID())
}

func Test2b(t *testing.T) {
	repo, _ := newTestRepo()
	gen := repo.GenesisBlock()
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(0))

	a1 := newBlock(0, inds[1:20], gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	b1 := newBlock(0, backers(), gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(
		0, inds[20:40], a1, 1, [4]thor.Bytes32{a1.Header().ID(), emptyID, b1.Header().ID()},
	)
	a3 := newBlock(0, inds[40:80], a2, 1, [4]thor.Bytes32{a1.Header().ID()})
	a4 := newBlock(0, backers(), a3, 1, [4]thor.Bytes32{GenNVForFirstBlock(4)})

	assert.Nil(t, repo.AddBlock(a1, nil))
	assert.Nil(t, repo.AddBlock(b1, nil))
	assert.Nil(t, repo.AddBlock(a2, nil))
	assert.Nil(t, repo.AddBlock(a3, nil))
	assert.Nil(t, repo.AddBlock(a4, nil))

	assert.Nil(t, repo.SetBestBlockID(a3.Header().ID()))
	assert.Nil(t, cons.Update(a3))
	assert.Equal(t, a1.Header().ID(), cons.state[NV])
	assert.True(t, cons.state[PP].IsZero())

	assert.Nil(t, repo.SetBestBlockID(a4.Header().ID()))
	assert.Nil(t, cons.Update(a4))
	assert.Equal(t, a4.Header().ID(), cons.state[NV])
	assert.True(t, cons.state[PP].IsZero())
}

func Test2c(t *testing.T) {
	repo, _ := newTestRepo()
	gen := repo.GenesisBlock()
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(0))

	a1 := newBlock(0, inds[1:20], gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(0, inds[20:40], a1, 1, [4]thor.Bytes32{a1.Header().ID()})
	a3 := newBlock(0, inds[40:80], a2, 1, [4]thor.Bytes32{a1.Header().ID()})
	a4 := newBlock(0, backers(), a3, 1, [4]thor.Bytes32{GenNVForFirstBlock(4)})
	a5 := newBlock(0, backers(), a4, 1, [4]thor.Bytes32{a4.Header().ID()})
	b5 := newBlock(0, backers(), a4, 1, [4]thor.Bytes32{GenNVForFirstBlock(5)})

	repo.AddBlock(a1, nil)
	repo.AddBlock(a2, nil)
	repo.AddBlock(a3, nil)
	repo.AddBlock(a4, nil)
	repo.AddBlock(a5, nil)
	repo.AddBlock(b5, nil)

	repo.SetBestBlockID(a3.Header().ID())
	assert.Nil(t, cons.Update(a3))
}

func Test3ai(t *testing.T) {
	repo, _ := newTestRepo()
	gen := repo.GenesisBlock()
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(0))

	pc := randBytes32()
	b := newBlock(0, backers(), gen, 3, [4]thor.Bytes32{emptyID, emptyID, pc})
	assert.Nil(t, cons.UpdateLastSignedPC(b))
	assert.Equal(t, pc, cons.lastSigned.Header().PC())
	assert.Equal(t, b.Header().Timestamp(), cons.lastSigned.Header().Timestamp())
}

func Test3b(t *testing.T) {
	repo, _ := newTestRepo()
	gen := repo.GenesisBlock()
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(0))

	a1 := newBlock(0, inds[1:20], gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(0, inds[20:40], a1, 1, [4]thor.Bytes32{a1.Header().ID()})
	a3 := newBlock(0, inds[40:80], a2, 1, [4]thor.Bytes32{a1.Header().ID()})
	a4 := newBlock(0, inds[1:20], a3, 1, [4]thor.Bytes32{GenNVForFirstBlock(4), a1.Header().ID()})
	a5 := newBlock(0, inds[20:40], a4, 1, [4]thor.Bytes32{a4.Header().ID(), a1.Header().ID(), randBytes32()})
	a6 := newBlock(0, inds[40:80], a5, 1, [4]thor.Bytes32{a4.Header().ID(), a1.Header().ID()})

	repo.AddBlock(a1, nil)
	repo.AddBlock(a2, nil)
	repo.AddBlock(a3, nil)
	repo.AddBlock(a4, nil)
	repo.AddBlock(a5, nil)
	repo.AddBlock(a6, nil)

	repo.SetBestBlockID(a3.Header().ID())
	assert.Nil(t, cons.Update(a3))
	repo.SetBestBlockID(a6.Header().ID())
	assert.Nil(t, cons.Update(a6))

	assert.True(t, cons.state[PC].IsZero())
	assert.Equal(t, cons.state[PP], a1.Header().ID())
}

func Test3c(t *testing.T) {
	repo, _ := newTestRepo()
	gen := repo.GenesisBlock()
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(0))

	a1 := newBlock(0, inds[1:20], gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(0, inds[20:40], a1, 1, [4]thor.Bytes32{a1.Header().ID()})
	a3 := newBlock(0, inds[40:80], a2, 1, [4]thor.Bytes32{a1.Header().ID()})
	a4 := newBlock(0, inds[1:20], a3, 1, [4]thor.Bytes32{GenNVForFirstBlock(4), a1.Header().ID()})
	a5 := newBlock(0, inds[20:40], a4, 1, [4]thor.Bytes32{a4.Header().ID(), a1.Header().ID()})
	a6 := newBlock(0, inds[40:80], a5, 1, [4]thor.Bytes32{a4.Header().ID(), a1.Header().ID()})
	a7 := newBlock(0, inds[1:20], a6, 1, [4]thor.Bytes32{GenNVForFirstBlock(7)})
	a8 := newBlock(0, inds[20:40], a7, 1, [4]thor.Bytes32{a7.Header().ID()})

	b8 := newBlock(0, inds[1:20], a7, 2, [4]thor.Bytes32{GenNVForFirstBlock(8)})
	b9 := newBlock(0, inds[20:40], b8, 1, [4]thor.Bytes32{b8.Header().ID()})
	b10 := newBlock(0, inds[40:80], b9, 1, [4]thor.Bytes32{b8.Header().ID()})

	repo.AddBlock(a1, nil)
	repo.AddBlock(a2, nil)
	repo.AddBlock(a3, nil)
	repo.AddBlock(a4, nil)
	repo.AddBlock(a5, nil)
	repo.AddBlock(a6, nil)
	repo.AddBlock(a7, nil)
	repo.AddBlock(a8, nil)

	repo.SetBestBlockID(a6.Header().ID())
	assert.Nil(t, cons.Update(a6))
	repo.SetBestBlockID(a8.Header().ID())
	assert.Nil(t, cons.Update(a8))
	assert.Equal(t, a7.Header().ID(), cons.state[NV])
	assert.Equal(t, a4.Header().ID(), cons.state[PP])
	assert.Equal(t, a1.Header().ID(), cons.state[PC])

	repo.AddBlock(b8, nil)
	repo.AddBlock(b9, nil)
	repo.AddBlock(b10, nil)
	repo.SetBestBlockID(b9.Header().ID())
	assert.Nil(t, cons.Update(b9))
	assert.Equal(t, b8.Header().ID(), cons.state[NV])
	assert.Equal(t, a1.Header().ID(), cons.state[PC])

	repo.SetBestBlockID(b10.Header().ID())
	assert.Nil(t, cons.Update(b10))
	assert.True(t, cons.state[PC].IsZero())
	assert.Equal(t, b8.Header().ID(), cons.state[PP])
}

func Test3cii(t *testing.T) {
	repo, _ := newTestRepo()
	gen := repo.GenesisBlock()
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(0))

	a1 := newBlock(0, inds[1:80], gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(0, inds[1:20], a1, 1, [4]thor.Bytes32{GenNVForFirstBlock(2), a1.Header().ID()})
	a3 := newBlock(0, inds[20:40], a2, 1, [4]thor.Bytes32{a2.Header().ID(), a1.Header().ID()})
	a4 := newBlock(0, inds[40:80], a3, 1, [4]thor.Bytes32{a2.Header().ID(), a1.Header().ID()})
	b4 := newBlock(0, inds[1:80], a3, 1, [4]thor.Bytes32{GenNVForFirstBlock(4)})

	repo.AddBlock(a1, nil)
	repo.AddBlock(a2, nil)
	repo.AddBlock(a3, nil)
	repo.AddBlock(a4, nil)
	repo.AddBlock(b4, nil)

	repo.SetBestBlockID(a4.Header().ID())
	assert.Nil(t, cons.Update(a4))
	assert.True(t, cons.state[PC].IsZero())
}

func Test3ciii(t *testing.T) {
	repo, _ := newTestRepo()
	gen := repo.GenesisBlock()
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(0))

	a1 := newBlock(0, inds[1:80], gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(0, inds[1:80], a1, 1, [4]thor.Bytes32{GenNVForFirstBlock(2), a1.Header().ID()})
	a3 := newBlock(0, backers(), a2, 2, [4]thor.Bytes32{GenNVForFirstBlock(3)})
	b3 := newBlock(0, inds[1:20], a2, 1, [4]thor.Bytes32{GenNVForFirstBlock(3)})
	b4 := newBlock(0, inds[20:80], b3, 2, [4]thor.Bytes32{b3.Header().ID()})

	repo.AddBlock(a1, nil)
	repo.AddBlock(a2, nil)
	repo.AddBlock(a3, nil)
	repo.AddBlock(b3, nil)

	assert.Nil(t, cons.Update(a2))
	assert.Nil(t, cons.Update(a3))
	assert.Equal(t, a1.Header().ID(), cons.state[PC])

	repo.AddBlock(b4, nil)
	assert.Nil(t, cons.Update(b4))
	assert.True(t, cons.state[PC].IsZero())
}

func Test3e(t *testing.T) {
	repo, _ := newTestRepo()
	gen := repo.GenesisBlock()
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(0))

	a1 := newBlock(0, inds[1:80], gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(0, inds[1:80], a1, 1, [4]thor.Bytes32{GenNVForFirstBlock(2), a1.Header().ID()})
	a3 := newBlock(0, backers(), a2, 1, [4]thor.Bytes32{GenNVForFirstBlock(3)})
	b3 := newBlock(0, inds[1:20], a2, 1, [4]thor.Bytes32{GenNVForFirstBlock(3)})
	b4 := newBlock(0, inds[20:80], b3, 1, [4]thor.Bytes32{b3.Header().ID(), emptyID, a1.Header().ID()})

	repo.AddBlock(a1, nil)
	repo.AddBlock(a2, nil)
	repo.AddBlock(a3, nil)

	assert.Nil(t, cons.Update(a2))
	assert.Nil(t, cons.Update(a3))

	repo.AddBlock(b3, nil)
	repo.AddBlock(b4, nil)
	assert.Nil(t, cons.Update(b3))
	assert.Nil(t, cons.Update(b4))
	assert.Equal(t, a1.Header().ID(), cons.state[PC])
}

func Test3ei(t *testing.T) {
	repo, _ := newTestRepo()
	gen := repo.GenesisBlock()
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(0))

	a1 := newBlock(0, inds[1:80], gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(0, inds[1:80], a1, 1, [4]thor.Bytes32{GenNVForFirstBlock(2), a1.Header().ID()})
	a3 := newBlock(0, backers(), a2, 1, [4]thor.Bytes32{GenNVForFirstBlock(3)})
	b3 := newBlock(0, inds[1:20], a2, 1, [4]thor.Bytes32{GenNVForFirstBlock(3)})
	b4 := newBlock(0, inds[20:80], b3, 1, [4]thor.Bytes32{b3.Header().ID(), emptyID, a1.Header().ID()})

	repo.AddBlock(a1, nil)
	repo.AddBlock(a2, nil)
	assert.Nil(t, cons.Update(a2))

	repo.AddBlock(b3, nil)
	repo.AddBlock(b4, nil)
	assert.Nil(t, cons.Update(b3))
	assert.Nil(t, cons.Update(b4))

	repo.AddBlock(a3, nil)
	assert.Nil(t, cons.Update(a3))
	assert.Equal(t, a1.Header().ID(), cons.state[PC])
}

func Test3fi(t *testing.T) {
	/**

	gen -- a1 -- a2 (signed)
			\
			 \------- b2
			   		  v:b2
	*/
	repo, _ := newTestRepo()
	gen := repo.GenesisBlock()

	u := 0
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(u))

	backers := backers()
	a1 := newBlock(1, backers, gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(u, backers, a1, 1, [4]thor.Bytes32{a1.Header().ID(), emptyID, randBytes32()})
	b2 := newBlock(1, inds[1:80], a1, 2, [4]thor.Bytes32{GenNVForFirstBlock(2)})

	repo.AddBlock(a1, nil)
	repo.AddBlock(a2, nil)
	repo.AddBlock(b2, nil)

	assert.Nil(t, cons.UpdateLastSignedPC(a2))
	assert.Equal(t, cons.lastSigned.Header().ID(), a2.Header().ID())
	assert.False(t, cons.hasLastSignedpPCExpired)

	assert.Nil(t, cons.Update(b2))
	assert.True(t, cons.hasLastSignedpPCExpired)
}

func Test3fii(t *testing.T) {
	/**
	       v:a1  v:a2		   v:a3
	gen -- a1 -- a2 ---------- a3 (signed)
					\
					 \-- b3 -------- b4
					 	 v:b3
	*/

	repo, _ := newTestRepo()
	gen := repo.GenesisBlock()

	u := 0
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(u))

	a1 := newBlock(1, inds[1:80], gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(1, inds[1:80], a1, 1, [4]thor.Bytes32{GenNVForFirstBlock(2), a1.Header().ID()})
	a3 := newBlock(u, backers(), a2, 2, [4]thor.Bytes32{GenNVForFirstBlock(3), a2.Header().ID(), a1.Header().ID()})
	b3 := newBlock(1, inds[1:20], a2, 1, [4]thor.Bytes32{GenNVForFirstBlock(3)})
	b4 := newBlock(1, inds[20:80], b3, 2, [4]thor.Bytes32{b3.Header().ID()})

	repo.AddBlock(a1, nil)
	repo.AddBlock(a2, nil)
	repo.AddBlock(a3, nil)
	repo.AddBlock(b3, nil)
	repo.AddBlock(b4, nil)

	assert.Nil(t, cons.UpdateLastSignedPC(a3))

	assert.Nil(t, cons.Update(b4))
	assert.Equal(t, a3.Header().ID(), cons.lastSigned.Header().ID())
	assert.False(t, cons.hasLastSignedpPCExpired)
}

func Test3fiii(t *testing.T) {
	/**
	       v:a1  v:a2		   v:a3
	gen -- a1 -- a2 ---------- a3 (signed)
					\
					 \-- b3 -------- b4 -- b5
					 	 v:b3			   v:b5
	*/

	repo, _ := newTestRepo()
	gen := repo.GenesisBlock()

	u := 0
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(u))

	a1 := newBlock(1, inds[1:80], gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(1, inds[1:80], a1, 1, [4]thor.Bytes32{GenNVForFirstBlock(2), a1.Header().ID()})
	a3 := newBlock(u, backers(), a2, 2, [4]thor.Bytes32{GenNVForFirstBlock(3), a2.Header().ID(), a1.Header().ID()})
	b3 := newBlock(1, inds[1:20], a2, 1, [4]thor.Bytes32{GenNVForFirstBlock(3)})
	b4 := newBlock(1, inds[20:80], b3, 2, [4]thor.Bytes32{b3.Header().ID()})
	b5 := newBlock(1, inds[1:80], b4, 1, [4]thor.Bytes32{GenNVForFirstBlock(5)})

	repo.AddBlock(a1, nil)
	repo.AddBlock(a2, nil)
	repo.AddBlock(a3, nil)
	repo.AddBlock(b3, nil)
	repo.AddBlock(b4, nil)
	repo.AddBlock(b5, nil)

	assert.Nil(t, cons.UpdateLastSignedPC(a3))

	assert.Nil(t, cons.Update(b4))
	assert.Equal(t, a3.Header().ID(), cons.lastSigned.Header().ID())
	assert.False(t, cons.hasLastSignedpPCExpired)

	assert.Nil(t, cons.Update(b5))
	assert.Equal(t, a3.Header().ID(), cons.lastSigned.Header().ID())
	assert.True(t, cons.hasLastSignedpPCExpired)
}

func Test3fiv(t *testing.T) {
	/**
	       v:a1  v:a2		   v:a3
	gen -- a1 -- a2 ---------- a3 (signed)
					\
					 \-- b3 -------- b4 -- b5
					 	 v:b3			   v:b5
	*/

	repo, _ := newTestRepo()
	gen := repo.GenesisBlock()

	u := 0
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(u))

	a1 := newBlock(1, inds[1:80], gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(1, inds[1:80], a1, 1, [4]thor.Bytes32{GenNVForFirstBlock(2), a1.Header().ID()})
	a3 := newBlock(u, backers(), a2, 2, [4]thor.Bytes32{GenNVForFirstBlock(3), a2.Header().ID(), a1.Header().ID()})
	b3 := newBlock(1, inds[1:20], a2, 1, [4]thor.Bytes32{GenNVForFirstBlock(3)})
	b4 := newBlock(1, inds[20:80], b3, 2, [4]thor.Bytes32{b3.Header().ID()})
	b5 := newBlock(1, inds[1:80], b4, 1, [4]thor.Bytes32{GenNVForFirstBlock(5), emptyID, a1.Header().ID()})

	repo.AddBlock(a1, nil)
	repo.AddBlock(a2, nil)
	repo.AddBlock(a3, nil)
	repo.AddBlock(b3, nil)
	repo.AddBlock(b4, nil)
	repo.AddBlock(b5, nil)

	assert.Nil(t, cons.UpdateLastSignedPC(a3))

	assert.Nil(t, cons.Update(b4))
	assert.Equal(t, a3.Header().ID(), cons.lastSigned.Header().ID())
	assert.False(t, cons.hasLastSignedpPCExpired)

	assert.Nil(t, cons.Update(b5))
	assert.Equal(t, a3.Header().ID(), cons.lastSigned.Header().ID())
	assert.False(t, cons.hasLastSignedpPCExpired)
}

func Test3fv(t *testing.T) {
	/**
	       v:a1  v:a2		   v:a3
	gen -- a1 -- a2 ---------- a3 (signed)
					\
					 \-- b3 -------- b4 -- b5
					 	 v:b3			   v:b5
	*/

	repo, _ := newTestRepo()
	gen := repo.GenesisBlock()

	u := 0
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(u))

	a1 := newBlock(1, inds[1:80], gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(1, inds[1:80], a1, 1, [4]thor.Bytes32{GenNVForFirstBlock(2), a1.Header().ID()})
	a3 := newBlock(u, backers(), a2, 2, [4]thor.Bytes32{GenNVForFirstBlock(3), a2.Header().ID(), a1.Header().ID()})
	b3 := newBlock(1, inds[1:20], a2, 1, [4]thor.Bytes32{GenNVForFirstBlock(3)})
	b4 := newBlock(1, inds[20:80], b3, 2, [4]thor.Bytes32{b3.Header().ID()})
	b5 := newBlock(u, inds[1:80], b4, 1, [4]thor.Bytes32{GenNVForFirstBlock(5), emptyID, a1.Header().ID()})

	repo.AddBlock(a1, nil)
	repo.AddBlock(a2, nil)
	repo.AddBlock(a3, nil)
	repo.AddBlock(b3, nil)
	repo.AddBlock(b4, nil)
	repo.AddBlock(b5, nil)

	assert.Nil(t, cons.UpdateLastSignedPC(a3))

	assert.Nil(t, cons.Update(b4))
	assert.Equal(t, a3.Header().ID(), cons.lastSigned.Header().ID())
	assert.False(t, cons.hasLastSignedpPCExpired)

	assert.Nil(t, cons.UpdateLastSignedPC(b5))
	assert.Nil(t, cons.Update(b5))
	assert.Equal(t, b5.Header().ID(), cons.lastSigned.Header().ID())
	assert.False(t, cons.hasLastSignedpPCExpired)
}

func Test3g(t *testing.T) {
	/**
				 									  signed
	        v:a1  				 v:a4				  v:a7
	gen --- a1 --- a2 --- a3 --- a4 --- a5 --- a6 --- a7
	  \
	   \---------- b1 --- b2 --- b3 --- b4 --- b5 --- b6 --- b7 --- b8
				   v:b1				    v:b4 				 v:b7
	*/
	var (
		u     int = 0
		state [4]thor.Bytes32
		rtpc  *block.Header
	)

	repo, _ := newTestRepo()
	g := repo.GenesisBlock()
	cons := NewConsensus(repo, repo.BestBlock().Header().ID(), nodeAddress(u))
	state[CM] = repo.BestBlock().Header().ID()

	update := func(b *block.Block, isBest bool) {
		repo.AddBlock(b, nil)
		if isBest {
			repo.SetBestBlockID(b.Header().ID())
		}
		if cons.IfUpdateLastSignedPC(b) {
			assert.Nil(t, cons.UpdateLastSignedPC(b))
		}
		assert.Nil(t, cons.Update(b))
	}
	checkStatus := func() {
		assert.Equal(t, state, cons.state)
		if rtpc == nil {
			assert.Nil(t, cons.rtpc.get())
		} else {
			if cons.rtpc.get() == nil {
				t.Fail()
			} else {
				assert.Equal(t, rtpc.ID(), cons.rtpc.get().ID())
			}
		}
	}

	// blk: 1, ts:10
	a1 := newBlock(1, inds[1:40], g, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	update(a1, true)
	state[NV] = a1.Header().ID()
	checkStatus()

	// blk: 1' > 1, ts:20
	b1 := newBlock(1, inds[1:20], g, 2, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	update(b1, true)
	state[NV] = b1.Header().ID()
	checkStatus()

	// blk: 2', ts:30
	b2 := newBlock(1, inds[20:40], b1, 1, [4]thor.Bytes32{b1.Header().ID()})
	update(b2, true)
	checkStatus()

	// blk: 2, ts:20
	a2 := newBlock(1, inds[40:60], a1, 1, [4]thor.Bytes32{a1.Header().ID()})
	update(a2, false)
	checkStatus()

	// blk: 3, ts:30
	a3 := newBlock(1, inds[60:80], a2, 1, [4]thor.Bytes32{a1.Header().ID()})
	update(a3, true)
	state[NV] = a3.Header().ID()
	state[PP] = a1.Header().ID()
	checkStatus()

	// blk: 4, ts:40
	a4 := newBlock(
		1, inds[1:20], a3, 1,
		[4]thor.Bytes32{GenNVForFirstBlock(a3.Header().Number() + 1), a1.Header().ID()},
	)
	update(a4, true)
	state[NV] = a4.Header().ID()
	checkStatus()

	// blk: 3', ts:40
	b3 := newBlock(1, inds[40:80], b2, 1, [4]thor.Bytes32{b1.Header().ID()})
	update(b3, false)
	checkStatus()

	// blk: 4' > 4, ts:50
	b4 := newBlock(
		1, inds[1:20], b3, 1,
		[4]thor.Bytes32{GenNVForFirstBlock(b3.Header().Number() + 1), b1.Header().ID()},
	)
	update(b4, true)
	state[NV] = b4.Header().ID()
	state[PP] = thor.Bytes32{} // PP unlock
	checkStatus()

	// blk: 5', ts:60
	b5 := newBlock(
		1, inds[20:40], b4, 1,
		[4]thor.Bytes32{b4.Header().ID(), b1.Header().ID()},
	)
	update(b5, true)
	checkStatus()

	// blk: 5, ts:50
	a5 := newBlock(1, inds[20:50], a4, 1, [4]thor.Bytes32{a4.Header().ID(), a1.Header().ID()})
	update(a5, false)
	checkStatus()

	// blk: 6, ts:60
	a6 := newBlock(1, inds[50:80], a5, 1, [4]thor.Bytes32{a4.Header().ID(), a1.Header().ID()})
	update(a6, true)
	state[NV] = a6.Header().ID()
	state[PP] = a4.Header().ID()
	state[PC] = a1.Header().ID()
	rtpc = a1.Header()
	checkStatus()

	// blk: 7, ts: 70
	a7 := newBlock(u, backers(), a6, 1, [4]thor.Bytes32{GenNVForFirstBlock(7), a4.Header().ID(), a1.Header().ID()})
	update(a7, true)
	state[NV] = a7.Header().ID()
	checkStatus()
	assert.Equal(t, cons.lastSigned.Header().PC(), a1.Header().ID())
	assert.Equal(t, cons.lastSigned.Header().Timestamp(), a7.Header().Timestamp())
	assert.False(t, cons.hasLastSignedpPCExpired)

	// blk: 6', ts:70
	b6 := newBlock(
		1, inds[40:80], b5, 1,
		[4]thor.Bytes32{b4.Header().ID(), b1.Header().ID()},
	)
	update(b6, false)
	state[PC] = thor.Bytes32{}
	rtpc = b1.Header()
	checkStatus()

	// blk: 7', ts: 80
	b7 := newBlock(
		1, inds[1:40], b6, 1,
		[4]thor.Bytes32{GenNVForFirstBlock(7), b4.Header().ID()},
	)
	update(b7, true)
	state[NV] = b7.Header().ID() // pc = 1 since lastSignedPC = 1 and has not expired
	state[PP] = thor.Bytes32{}
	checkStatus()

	// blk: 8', ts: 90
	b8 := newBlock(1, inds[40:80], b7, 1, [4]thor.Bytes32{b7.Header().ID(), b4.Header().ID()})
	update(b8, true)
	state[PP] = b7.Header().ID()
	state[PC] = b4.Header().ID()
	rtpc = b4.Header()
	checkStatus()
}

func Test3gi(t *testing.T) {
	/**
				 									  signed
	        v:a1  				 v:a4				  v:a7
	gen --- a1 --- a2 --- a3 --- a4 --- a5 --- a6 --- a7
	  \
	   \---------- b1 --- b2 --- b3 --- b4 --- b5 --- b6 --- b7 --- b8
				   v:b1				    v:b4 				 v:b7
				   											 pc:a1
	*/
	var (
		u     int = 0
		state [4]thor.Bytes32
		rtpc  *block.Header
	)

	repo, _ := newTestRepo()
	g := repo.GenesisBlock()
	cons := NewConsensus(repo, repo.BestBlock().Header().ID(), nodeAddress(u))
	state[CM] = repo.BestBlock().Header().ID()

	update := func(b *block.Block, isBest bool) {
		repo.AddBlock(b, nil)
		if isBest {
			repo.SetBestBlockID(b.Header().ID())
		}
		if cons.IfUpdateLastSignedPC(b) {
			assert.Nil(t, cons.UpdateLastSignedPC(b))
		}
		assert.Nil(t, cons.Update(b))
	}
	checkStatus := func() {
		assert.Equal(t, state, cons.state)
		if rtpc == nil {
			assert.Nil(t, cons.rtpc.get())
		} else {
			if cons.rtpc.get() == nil {
				panic("RTPC not consistent")
			} else {
				assert.Equal(t, rtpc.ID(), cons.rtpc.get().ID())
			}
		}
	}

	// blk: 1, ts:10
	a1 := newBlock(1, inds[1:40], g, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	update(a1, true)
	state[NV] = a1.Header().ID()
	checkStatus()

	// blk: 1' > 1, ts:20
	b1 := newBlock(1, inds[1:20], g, 2, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	update(b1, true)
	state[NV] = b1.Header().ID()
	checkStatus()

	// blk: 2', ts:30
	b2 := newBlock(1, inds[20:40], b1, 1, [4]thor.Bytes32{b1.Header().ID()})
	update(b2, true)
	checkStatus()

	// blk: 2, ts:20
	a2 := newBlock(1, inds[40:60], a1, 1, [4]thor.Bytes32{a1.Header().ID()})
	update(a2, false)
	checkStatus()

	// blk: 3, ts:30
	a3 := newBlock(1, inds[60:80], a2, 1, [4]thor.Bytes32{a1.Header().ID()})
	update(a3, true)
	state[NV] = a3.Header().ID()
	state[PP] = a1.Header().ID()
	checkStatus()

	// blk: 4, ts:40
	a4 := newBlock(
		1, inds[1:20], a3, 1,
		[4]thor.Bytes32{GenNVForFirstBlock(a3.Header().Number() + 1), a1.Header().ID()},
	)
	update(a4, true)
	state[NV] = a4.Header().ID()
	checkStatus()

	// blk: 3', ts:40
	b3 := newBlock(1, inds[40:80], b2, 1, [4]thor.Bytes32{b1.Header().ID()})
	update(b3, false)
	checkStatus()

	// blk: 4' > 4, ts:50
	b4 := newBlock(
		1, inds[1:20], b3, 1,
		[4]thor.Bytes32{GenNVForFirstBlock(b3.Header().Number() + 1), b1.Header().ID()},
	)
	update(b4, true)
	state[NV] = b4.Header().ID()
	state[PP] = thor.Bytes32{} // PP unlock
	checkStatus()

	// blk: 5', ts:60
	b5 := newBlock(
		1, inds[20:40], b4, 1,
		[4]thor.Bytes32{b4.Header().ID(), b1.Header().ID()},
	)
	update(b5, true)
	checkStatus()

	// blk: 5, ts:50
	a5 := newBlock(1, inds[20:50], a4, 1, [4]thor.Bytes32{a4.Header().ID(), a1.Header().ID()})
	update(a5, false)
	checkStatus()

	// blk: 6, ts:60
	a6 := newBlock(1, inds[50:80], a5, 1, [4]thor.Bytes32{a4.Header().ID(), a1.Header().ID()})
	update(a6, true)
	state[NV] = a6.Header().ID()
	state[PP] = a4.Header().ID()
	state[PC] = a1.Header().ID()
	rtpc = a1.Header()
	checkStatus()

	// blk: 7, ts: 70
	a7 := newBlock(u, backers(), a6, 1, [4]thor.Bytes32{GenNVForFirstBlock(7), a4.Header().ID(), a1.Header().ID()})
	update(a7, true)
	state[NV] = a7.Header().ID()
	checkStatus()
	assert.Equal(t, cons.lastSigned.Header().ID(), a7.Header().ID())
	assert.False(t, cons.hasLastSignedpPCExpired)

	// blk: 6', ts:70
	b6 := newBlock(
		1, inds[40:80], b5, 1,
		[4]thor.Bytes32{b4.Header().ID(), b1.Header().ID()},
	)
	update(b6, false)
	state[PC] = thor.Bytes32{}
	rtpc = b1.Header()
	checkStatus()

	// blk: 7', ts: 80
	b7 := newBlock(
		1, inds[1:40], b6, 1,
		[4]thor.Bytes32{GenNVForFirstBlock(7), b4.Header().ID()},
	)
	update(b7, true)
	state[NV] = b7.Header().ID() // pc = 1 since lastSignedPC = 1 and has not expired
	state[PP] = thor.Bytes32{}
	checkStatus()

	// blk: 8', ts: 90
	b8 := newBlock(
		1, inds[40:80], b7, 1,
		[4]thor.Bytes32{b7.Header().ID(), b4.Header().ID(), a1.Header().ID()},
	)
	update(b8, true)
	rtpc = nil
	assert.Equal(t, cons.lastSigned.Header().ID(), a7.Header().ID())
	assert.False(t, cons.hasLastSignedpPCExpired)
	checkStatus()
}

func Test3gii(t *testing.T) {
	/**
				 									  signed
	        v:a1  				 v:a4				  v:a7
	gen --- a1 --- a2 --- a3 --- a4 --- a5 --- a6 --- a7
	  \
	   \---------- b1 --- b2 --- b3 --- b4 --- b5 --- b6 --- b7 --- b8
				   v:b1				    v:b4 				 v:b7
				   											 pc:a1
	*/
	var (
		u     int = 100
		state [4]thor.Bytes32
		rtpc  *block.Header
	)

	repo, _ := newTestRepo()
	g := repo.GenesisBlock()
	cons := NewConsensus(repo, repo.BestBlock().Header().ID(), nodeAddress(u))
	state[CM] = repo.BestBlock().Header().ID()

	update := func(b *block.Block, isBest bool) {
		repo.AddBlock(b, nil)
		if isBest {
			repo.SetBestBlockID(b.Header().ID())
		}
		if cons.IfUpdateLastSignedPC(b) {
			assert.Nil(t, cons.UpdateLastSignedPC(b))
		}
		assert.Nil(t, cons.Update(b))
	}
	checkStatus := func() {
		assert.Equal(t, state, cons.state)
		if rtpc == nil {
			assert.Nil(t, cons.rtpc.get())
		} else {
			if cons.rtpc.get() == nil {
				panic("RTPC not consistent")
			} else {
				assert.Equal(t, rtpc.ID(), cons.rtpc.get().ID())
			}
		}
	}

	a1 := newBlock(1, inds[1:40], g, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})

	b1 := newBlock(1, inds[1:20], g, 2, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	update(b1, true)
	b2 := newBlock(1, inds[20:40], b1, 1, [4]thor.Bytes32{b1.Header().ID()})
	update(b2, true)
	b3 := newBlock(1, inds[40:80], b2, 1, [4]thor.Bytes32{b1.Header().ID()})
	update(b3, true)
	b4 := newBlock(
		1, inds[1:20], b3, 1,
		[4]thor.Bytes32{GenNVForFirstBlock(4), b1.Header().ID()},
	)
	update(b4, true)
	b5 := newBlock(
		1, inds[20:40], b4, 1,
		[4]thor.Bytes32{b4.Header().ID(), b1.Header().ID()},
	)
	update(b5, true)
	b6 := newBlock(
		1, inds[40:80], b5, 1,
		[4]thor.Bytes32{b4.Header().ID(), b1.Header().ID(), a1.Header().ID()},
	)
	update(b6, true)
	b7 := newBlock(
		1, inds[1:40], b6, 1,
		[4]thor.Bytes32{GenNVForFirstBlock(7), b4.Header().ID(), a1.Header().ID()},
	)
	update(b7, true)

	state[NV] = b7.Header().ID()
	state[PP] = b1.Header().ID()
	checkStatus()

	update(a1, false)
	a2 := newBlock(1, inds[40:60], a1, 1, [4]thor.Bytes32{a1.Header().ID()})
	update(a2, false)
	a3 := newBlock(1, inds[60:80], a2, 1, [4]thor.Bytes32{a1.Header().ID()})
	update(a3, false)
	a4 := newBlock(
		1, inds[1:20], a3, 1,
		[4]thor.Bytes32{GenNVForFirstBlock(a3.Header().Number() + 1), a1.Header().ID()},
	)
	update(a4, false)
	a5 := newBlock(1, inds[20:50], a4, 1, [4]thor.Bytes32{a4.Header().ID(), a1.Header().ID()})
	update(a5, false)
	a6 := newBlock(1, inds[50:80], a5, 1, [4]thor.Bytes32{a4.Header().ID(), a1.Header().ID()})
	update(a6, false)
	a7 := newBlock(u, backers(), a6, 1, [4]thor.Bytes32{GenNVForFirstBlock(7), a4.Header().ID(), a1.Header().ID()})
	update(a7, false)

	state[PC] = a1.Header().ID()
	rtpc = a1.Header()
	checkStatus()
}

func Test4b(t *testing.T) {
	repo, _ := newTestRepo()
	gen := repo.BestBlock()
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(0))

	update := func(b *block.Block, isBest bool) {
		repo.AddBlock(b, nil)
		if isBest {
			repo.SetBestBlockID(b.Header().ID())
		}
		if cons.IfUpdateLastSignedPC(b) {
			assert.Nil(t, cons.UpdateLastSignedPC(b))
		}
		assert.Nil(t, cons.Update(b))
	}

	a1 := newBlock(0, inds[1:80], gen, 1, [4]thor.Bytes32{GenNVForFirstBlock(1)})
	a2 := newBlock(0, inds[1:80], a1, 1, [4]thor.Bytes32{GenNVForFirstBlock(2), a1.Header().ID()})
	a3 := newBlock(0, inds[1:20], a2, 1, [4]thor.Bytes32{GenNVForFirstBlock(3), a2.Header().ID(), a1.Header().ID()})
	a4 := newBlock(0, inds[20:80], a3, 1, [4]thor.Bytes32{a3.Header().ID(), a2.Header().ID(), randBytes32()})

	update(a1, true)
	update(a2, true)
	update(a3, true)
	update(a4, true)

	assert.Equal(t, gen.Header().ID(), cons.state[CM])
}

func TestCM(t *testing.T) {
	repo, _ := newTestRepo()
	gen := repo.BestBlock()
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(0))

	leader := 0
	backers := inds[:QC]

	var blocks []*block.Block
	blocks = append(blocks, gen)

	for i := 1; i <= 3; i++ {
		fv := [4]thor.Bytes32{GenNVForFirstBlock(uint32(i))}
		if i == 3 {
			fv = [4]thor.Bytes32{
				GenNVForFirstBlock(uint32(i)),
				emptyID,
				emptyID,
				blocks[2].Header().ID(),
			}
		}

		b := newBlock(leader, backers, blocks[i-1], 1, fv)
		assert.Nil(t, repo.AddBlock(b, nil))
		blocks = append(blocks, b)
	}

	// Observed CM not newer than the locally committed
	cons.state[CM] = blocks[3].Header().ID()
	assert.Nil(t, cons.Update(blocks[3]))
	assert.Equal(t, blocks[3].Header().ID(), cons.state[CM])

	// Observed CM newer than the locally committed
	cons.state = [4]thor.Bytes32{}
	cons.state[CM] = blocks[1].Header().ID()
	assert.Nil(t, cons.Update(blocks[3]))
	assert.Equal(t, blocks[2].Header().ID(), cons.state[CM])
}

func TestNVHasQC(t *testing.T) {
	/**
	Genesis --- b1 --- b2

	b1 has a QC
	nv[b1] <- b1
	nv[b2] <- b1
	*/

	repo, _ := newTestRepo()
	gen := repo.BestBlock()
	cons := NewConsensus(repo, gen.Header().ID(), nodeAddress(0))

	b1 := newBlock(
		0, inds[:QC+3], gen, 1,
		[4]thor.Bytes32{GenNVForFirstBlock(1)},
	)

	b2 := newBlock(
		0, backers(), b1, 1,
		[4]thor.Bytes32{b1.Header().ID()},
	)

	assert.Nil(t, repo.AddBlock(b1, nil))
	assert.Nil(t, repo.AddBlock(b2, nil))

	assert.Nil(t, cons.repo.SetBestBlockID(b2.Header().ID()))

	cons.state[NV] = b1.Header().ID()
	assert.Nil(t, cons.Update(b2))
	assert.Equal(t, b2.Header().ID(), cons.state[NV])
}
