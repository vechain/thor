package bft

import (
	rnd "math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

var (
	nNode        = 101
	nBlock       = 10
	maxNumBacker = 5
	randID       = randBytes32()
	viewStart    = 2
	viewEnd      = 6
	randThres    = 3

	emptyRootHash = new(tx.Transactions).RootHash()
)

func TestNewRandBlock(t *testing.T) {
	repo, _ := newTestRepo()

	proposer := rnd.Intn(nNode)
	backers := []int{}
	for i := 0; i < 5; i++ {
		backers = append(backers, rnd.Intn(nNode))
	}

	b := newBlock(proposer, backers, repo.GenesisBlock(), 1, [4]thor.Bytes32{})
	es := getSigners(b)

	addrs := make(map[thor.Address]bool)
	for _, e := range es {
		addrs[e] = true
	}

	assert.True(t, addrs[nodeAddress(proposer)])
	for _, backer := range backers {
		assert.True(t, addrs[nodeAddress(backer)])
	}
}

func newTestBranch(repo *chain.Repository) (
	nodeInds [][]int, blks []*block.Block, fvInds [][4]int,
) {
	nodeInds = make([][]int, nBlock+1)
	blks = make([]*block.Block, nBlock+1)
	fvInds = make([][4]int, nBlock+1)

	blks[0] = repo.GenesisBlock()

	// Sample proposers+backers for blocks
	for i := 1; i <= nBlock; i++ {
		n := rnd.Intn(maxNumBacker+1) + 1
		nodeInds[i] = make([]int, n)
		for j := 0; j < n; j++ {
			nodeInds[i][j] = rnd.Intn(nNode)
		}
	}

	// Sample finality vectors for blocks
	for i := 1; i <= nBlock; i++ {
		for j := 0; j < 4; j++ {
			fvInds[i][j] = rnd.Intn(i)
			if rnd.Intn(10) < randThres {
				fvInds[i][j] = -1
			}
		}
	}
	for i := viewStart + 1; i <= viewEnd; i++ {
		fvInds[i][0] = 2
	}
	fvInds[viewEnd+1][0] = 1 // make sure that view ends at block with number of viewEnd

	// Construct the branch
	for i := 1; i <= nBlock; i++ {
		var (
			fv       [4]thor.Bytes32
			proposer = nodeInds[i][0]
			backers  = nodeInds[i][1:]
		)

		for j, val := range fvInds[i] {
			if val < 0 {
				fv[j] = randID
			} else {
				fv[j] = blks[val].Header().ID()
			}
		}
		if i == viewStart { // first block of view
			nv := GenNVForFirstBlock(uint32(viewStart))
			fv[0] = nv
		}

		blks[i] = newBlock(proposer, backers, blks[i-1], 1, fv)
		if err := repo.AddBlock(blks[i], nil); err != nil {
			panic(err)
		}
	}

	return
}

func TestNewView(t *testing.T) {
	repo, _ := newTestRepo()
	nodeInds, blks, fvInds := newTestBranch(repo)

	branch := repo.NewChain(blks[len(blks)-1].Header().ID())

	vw, _ := newView(branch, block.Number(randBytes32()))
	assert.Nil(t, vw) // block with random nv value
	vw, _ = newView(branch, block.Number(blks[1].Header().ID()))
	assert.Nil(t, vw) // block with invalid nv value

	vw, _ = newView(branch, block.Number(blks[2].Header().ID()))

	assert.Equal(t, vw.getFirstBlockID(), blks[2].Header().ID()) // verify id of the first block in the view

	// verify whether the view has any conflict pc
	hasConflictPC := false
	for i := viewStart; i <= viewEnd; i++ {
		if fvInds[i][2] < 0 {
			hasConflictPC = true
			break
		}
	}
	assert.Equal(t, vw.hasConflictPC(), hasConflictPC)

	// verify nv info
	nvs := []int(nil)
	for i := viewStart; i <= viewEnd; i++ {
		nvs = append(nvs, nodeInds[i]...)
	}
	nvSummary := countIntArray(nvs)
	for i, val := range nvSummary {
		// addr := thor.Address(crypto.PubkeyToAddress(nodes[i].PublicKey))
		addr := nodeAddress(i)
		assert.Equal(t, vw.nv[addr], uint8(val))
	}

	// verify pp
	pps := make(map[int][]int)
	for i := viewStart; i <= viewEnd; i++ {
		pp := fvInds[i][1]
		pps[pp] = append(pps[pp], nodeInds[i]...)
	}
	for key, val := range pps {
		var id thor.Bytes32
		if key == -1 {
			id = randID
		} else {
			id = blks[key].Header().ID()
		}
		summary := countIntArray(val)
		for i, j := range summary {
			// addr := thor.Address(crypto.PubkeyToAddress(keys[i].PublicKey))
			addr := nodeAddress(i)
			expected := uint8(j)
			actual := vw.pp[id][addr]
			assert.Equal(t, expected, actual)
		}
	}

	// verify pc
	pcs := make(map[int][]int)
	for i := viewStart; i <= viewEnd; i++ {
		pc := fvInds[i][2]
		pcs[pc] = append(pcs[pc], nodeInds[i]...)
	}
	for key, val := range pcs {
		var id thor.Bytes32
		if key == -1 {
			id = randID
		} else {
			id = blks[key].Header().ID()
		}
		summary := countIntArray(val)
		for i, j := range summary {
			// addr := thor.Address(crypto.PubkeyToAddress(keys[i].PublicKey))
			addr := nodeAddress(i)
			expected := uint8(j)
			actual := vw.pc[id][addr]
			assert.Equal(t, expected, actual)
		}
	}
}

func TestViewFunc(t *testing.T) {
	// 		b0
	// 		|
	//		|
	//		b1
	//		|--------
	// 		|		|
	// 		b2 		b3

	repo, _ := newTestRepo()

	var (
		pp = randBytes32()
		pc = randBytes32()
	)

	blk1 := newBlock(
		0, inds[1:30], repo.GenesisBlock(), 1,
		[4]thor.Bytes32{GenNVForFirstBlock(1), pp, pc},
	)
	assert.Nil(t, repo.AddBlock(blk1, nil))

	blk2 := newBlock(
		30, inds[31:68], blk1, 1,
		[4]thor.Bytes32{blk1.Header().ID(), pp, pc},
	)

	blk3 := newBlock(
		30, inds[31:66], blk1, 1,
		[4]thor.Bytes32{blk1.Header().ID(), randID},
	)
	assert.Nil(t, repo.AddBlock(blk2, nil))
	assert.Nil(t, repo.AddBlock(blk3, nil))

	var (
		bhs []*chain.Chain
		vw  *view
	)
	bhs, _ = repo.GetBranchesByID(blk2.Header().ID())
	vw, _ = newView(bhs[0], block.Number(blk1.Header().ID()))
	assert.True(t, vw.hasQCForNV())
	assert.Equal(t, M(true, pp), M(vw.hasQCForPP()))
	assert.Equal(t, M(true, pc), M(vw.hasQCForPC()))

	bhs, _ = repo.GetBranchesByID(blk3.Header().ID())
	vw, _ = newView(bhs[0], block.Number(blk1.Header().ID()))
	assert.False(t, vw.hasQCForNV())
	assert.Equal(t, M(false, thor.Bytes32{}), M(vw.hasQCForPP()))
	assert.Equal(t, M(false, thor.Bytes32{}), M(vw.hasQCForPC()))
	assert.Equal(t, 2, len(vw.pp))
	assert.Equal(t, 1, len(vw.pc))
	assert.Equal(t, 30, len(vw.pp[pp]))
	assert.Equal(t, 36, len(vw.pp[randID]))

	// Test getNumSigOnPC
	assert.Equal(t, 30, vw.getNumSigOnPC(pc))
	assert.Equal(t, 0, vw.getNumSigOnPC(randBytes32()))
}
