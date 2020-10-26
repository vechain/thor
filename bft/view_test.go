package bft

import (
	"crypto/ecdsa"
	"crypto/rand"
	rnd "math/rand"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
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

func newBlock(
	proposer *ecdsa.PrivateKey,
	backers []*ecdsa.PrivateKey,
	parentID thor.Bytes32,
	timestamp uint64,
	gasLimit uint64,
	fv [4]thor.Bytes32,
) (blk *block.Block) {

	msg := block.NewProposal(
		parentID, emptyRootHash, gasLimit, timestamp,
	).AsMessage(thor.Address(crypto.PubkeyToAddress(proposer.PublicKey)))

	bss := block.ComplexSignatures(nil)
	for _, backer := range backers {
		proof := make([]byte, 81)
		rand.Read(proof)
		sig, err := crypto.Sign(thor.Blake2b(msg, proof).Bytes(), backer)
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
		ParentID(parentID).
		Timestamp(timestamp).
		GasLimit(gasLimit).
		BackerSignatures(bss, 0, 0).
		FinalityVector(fv)

	blk = builder.Build()
	sig, err := crypto.Sign(blk.Header().SigningHash().Bytes(), proposer)
	if err != nil {
		panic(err)
	}
	blk = blk.WithSignature(sig)

	return
}

func newTestRepo() *chain.Repository {
	db := muxdb.NewMem()
	g := genesis.NewDevnet()
	b0, _, _, _ := g.Build(state.NewStater(db))

	repo, err := chain.NewRepository(db, b0)
	if err != nil {
		panic(err)
	}
	return repo
}

func pubToAddr(pub ecdsa.PublicKey) thor.Address {
	return thor.Address(crypto.PubkeyToAddress(pub))
}

func TestNewRandBlock(t *testing.T) {
	proposer, _ := crypto.GenerateKey()
	backers := make([]*ecdsa.PrivateKey, 1)
	for i := 0; i < 1; i++ {
		backers[i], _ = crypto.GenerateKey()
	}
	b := newBlock(proposer, backers, thor.Bytes32{}, 0, 0, [4]thor.Bytes32{})
	es := getSigners(b)

	addrs := make(map[thor.Address]bool)
	for _, e := range es {
		addrs[e] = true
	}

	assert.True(t, addrs[pubToAddr(proposer.PublicKey)])
	for _, backer := range backers {
		assert.True(t, addrs[pubToAddr(backer.PublicKey)])
	}
}

func newTestBranch(repo *chain.Repository, keys []*ecdsa.PrivateKey) (
	nodeInds [][]int, blks []*block.Block, fvInds [][4]int,
) {
	// nodes that sign a block
	// nodes[][0] is the proposer, nodes[][1:] are backers
	nodeInds = make([][]int, nBlock+1)
	blks = make([]*block.Block, nBlock+1)
	fvInds = make([][4]int, nBlock+1)

	blks[0] = repo.GenesisBlock()

	// Init proposers/backers for blocks
	for i := 1; i <= nBlock; i++ {
		n := rnd.Intn(maxNumBacker+1) + 1
		nodeInds[i] = make([]int, n)
		for j := 0; j < n; j++ {
			nodeInds[i][j] = rnd.Intn(nNode)
		}
	}

	// Init finality vectors for blocks
	//
	// test view: block viewStart ~ viewEnd
	// negative value == random id == conflict block
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
		nodes := make([]*ecdsa.PrivateKey, len(nodeInds[i]))
		for j, val := range nodeInds[i] {
			nodes[j] = keys[val]
		}

		var (
			parentID  thor.Bytes32
			timestamp uint64
			fv        [4]thor.Bytes32
		)
		parentID = blks[i-1].Header().ID()
		timestamp = blks[i-1].Header().Timestamp() + 10

		for j, val := range fvInds[i] {
			if val < 0 {
				fv[j] = randID
			} else {
				fv[j] = blks[val].Header().ID()
			}
		}
		if i == viewStart { // first block of view
			// nv := thor.Bytes32{}
			// binary.BigEndian.PutUint32(nv[:], uint32(viewStart))
			nv := genNVforFirstBlock(uint32(viewStart))
			fv[0] = nv
		}

		blks[i] = newBlock(nodes[0], nodes[1:], parentID, timestamp, 0, fv)
		if err := repo.AddBlock(blks[i], nil); err != nil {
			panic(err)
		}
	}

	return
}

func TestNewView(t *testing.T) {
	// Generate private keys for nodes
	keys := []*ecdsa.PrivateKey(nil)
	for i := 0; i < nNode; i++ {
		key, _ := crypto.GenerateKey()
		keys = append(keys, key)
	}

	for count := 0; count < 10; count++ {
		repo := newTestRepo()
		nodeInds, blks, fvInds := newTestBranch(repo, keys)

		branches := repo.GetBranches(repo.GenesisBlock().Header().ID())

		assert.Equal(t, len(branches), 1)                                      // only one branch
		assert.Equal(t, branches[0].HeadID(), blks[len(blks)-1].Header().ID()) // verify branch head

		assert.Nil(t, newView(branches[0], randBytes32()))         // block not on chain
		assert.Nil(t, newView(branches[0], blks[1].Header().ID())) // block with invalid nv value

		v := newView(branches[0], blks[2].Header().ID())

		assert.Equal(t, v.first, blks[2].Header().ID()) // verify id of the first block in the view

		// verify whether the view has any conflict pc
		hasConflictPC := false
		for i := viewStart; i <= viewEnd; i++ {
			if fvInds[i][2] < 0 {
				hasConflictPC = true
			}
		}
		assert.Equal(t, v.hasConflictPC, hasConflictPC)

		// verify nv info
		nvs := []int(nil)
		for i := viewStart; i <= viewEnd; i++ {
			nvs = append(nvs, nodeInds[i]...)
		}
		nvSummary := countIntArray(nvs)
		for i, val := range nvSummary {
			addr := thor.Address(crypto.PubkeyToAddress(keys[i].PublicKey))
			assert.Equal(t, v.nv[addr], uint8(val))
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
				addr := thor.Address(crypto.PubkeyToAddress(keys[i].PublicKey))
				expected := uint8(j)
				actual := v.pp[id][addr]
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
				addr := thor.Address(crypto.PubkeyToAddress(keys[i].PublicKey))
				expected := uint8(j)
				actual := v.pc[id][addr]
				assert.Equal(t, expected, actual)
			}
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

	// Generate private keys for nodes
	keys := []*ecdsa.PrivateKey(nil)
	for i := 0; i < nNode; i++ {
		key, _ := crypto.GenerateKey()
		keys = append(keys, key)
	}

	repo := newTestRepo()
	gen := repo.GenesisBlock()

	var (
		pp = randBytes32()
		pc = randBytes32()
	)

	blk1 := newBlock(
		keys[0],
		keys[1:30],
		gen.Header().ID(),
		gen.Header().Timestamp()+10,
		0,
		[4]thor.Bytes32{
			genNVforFirstBlock(1),
			pp,
			pc,
			thor.Bytes32{},
		},
	)

	blk2 := newBlock(
		keys[30],
		keys[31:68],
		blk1.Header().ID(),
		blk1.Header().Timestamp()+10,
		0,
		[4]thor.Bytes32{
			blk1.Header().ID(),
			pp,
			pc,
			thor.Bytes32{},
		},
	)

	blk3 := newBlock(
		keys[30],
		keys[31:66],
		blk1.Header().ID(),
		blk1.Header().Timestamp()+10,
		0,
		[4]thor.Bytes32{
			blk1.Header().ID(),
			randID,
			thor.Bytes32{},
			thor.Bytes32{},
		},
	)

	assert.Nil(t, repo.AddBlock(blk1, nil))
	assert.Nil(t, repo.AddBlock(blk2, nil))
	assert.Nil(t, repo.AddBlock(blk3, nil))

	var (
		bh *chain.Chain
		vw *view
	)
	bh = repo.GetBranches(blk2.Header().ID())[0]
	vw = newView(bh, blk1.Header().ID())
	assert.True(t, vw.ifHasQCForNV())
	assert.Equal(t, M(true, pp), M(vw.ifHasQCForPP()))
	assert.Equal(t, M(true, pc), M(vw.ifHasQCForPC()))

	bh = repo.GetBranches(blk3.Header().ID())[0]
	vw = newView(bh, blk1.Header().ID())
	assert.False(t, vw.ifHasQCForNV())
	assert.Equal(t, M(false, thor.Bytes32{}), M(vw.ifHasQCForPP()))
	assert.Equal(t, M(false, thor.Bytes32{}), M(vw.ifHasQCForPC()))
	assert.Equal(t, 2, len(vw.pp))
	assert.Equal(t, 1, len(vw.pc))
	assert.Equal(t, 30, len(vw.pp[pp]))
	assert.Equal(t, 36, len(vw.pp[randID]))
	assert.Equal(t, 30, len(vw.pc[pc]))
}
