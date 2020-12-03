package bft

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/go-ecvrf"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var (
	dbs     []*muxdb.MuxDB
	repos   []*chain.Repository
	bftCons []*Consensus

	a1, a2, a3, a4, a5, a6 *block.Block
	b1, b2, b3, b4, b5, b6 *block.Block
	c1, c2, c3, c4, c5, c6 *block.Block
)

func randQC() []int {
	return randNums(nNode-1, QC)
}

func resetVars() {
	dbs = []*muxdb.MuxDB{}
	repos = []*chain.Repository{}
	bftCons = []*Consensus{}
	a1 = nil
	a2 = nil
	a3 = nil
	a4 = nil
	a5 = nil
	a6 = nil

	b1 = nil
	b2 = nil
	b3 = nil
	b4 = nil
	b5 = nil
	b6 = nil

	c1 = nil
	c2 = nil
	c3 = nil
	c4 = nil
	c5 = nil
	c6 = nil
}

func initNodesStatus(t *testing.T) {
	resetVars()

	t1 := int(QC / 3)
	t2 := int(2 * QC / 3)

	// Init repository and consensus for nodes
	for i := 0; i < 3; i++ {
		repo, db := newTestRepo()
		dbs = append(dbs, db)
		repos = append(repos, repo)
		bftCons = append(bftCons, NewConsensus(repos[i], repos[i].GenesisBlock().Header().ID(), nodeAddress(i)))
	}

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

	// node 0-32
	repos[0].AddBlock(a1, nil)
	repos[0].AddBlock(a2, nil)
	repos[0].AddBlock(a3, nil)
	repos[0].AddBlock(a4, nil)
	repos[0].AddBlock(a5, nil)
	repos[0].AddBlock(a6, nil)
	repos[0].AddBlock(b1, nil)
	repos[0].AddBlock(b2, nil)
	repos[0].AddBlock(b3, nil)
	repos[0].AddBlock(b4, nil)
	repos[0].AddBlock(b5, nil)
	repos[0].AddBlock(c1, nil)
	repos[0].AddBlock(c2, nil)
	repos[0].AddBlock(c3, nil)
	repos[0].AddBlock(c4, nil)

	bftCons[0].repo.SetBestBlockID(a6.Header().ID())
	assert.Nil(t, bftCons[0].Update(a6))

	assert.Equal(t, a4.Header().ID(), bftCons[0].state[NV])
	assert.Equal(t, a4.Header().ID(), bftCons[0].state[PP])
	assert.Equal(t, a1.Header().ID(), bftCons[0].state[PC])

	// node 33-65
	repos[1].AddBlock(a1, nil)
	repos[1].AddBlock(a2, nil)
	repos[1].AddBlock(a3, nil)
	repos[1].AddBlock(a4, nil)
	repos[1].AddBlock(a5, nil)
	repos[1].AddBlock(a6, nil)
	repos[1].AddBlock(b1, nil)
	repos[1].AddBlock(b2, nil)
	repos[1].AddBlock(b3, nil)
	repos[1].AddBlock(b4, nil)
	repos[1].AddBlock(b5, nil)
	repos[1].AddBlock(b6, nil)
	repos[1].AddBlock(c1, nil)
	repos[1].AddBlock(c2, nil)
	repos[1].AddBlock(c3, nil)
	repos[1].AddBlock(c4, nil)
	repos[1].AddBlock(c5, nil)

	bftCons[1].repo.SetBestBlockID(b6.Header().ID())
	assert.Nil(t, bftCons[1].Update(b6))

	assert.Equal(t, b4.Header().ID(), bftCons[1].state[NV])
	assert.Equal(t, b4.Header().ID(), bftCons[1].state[PP])
	assert.Equal(t, b1.Header().ID(), bftCons[1].state[PC])

	// node 66-100
	repos[2].AddBlock(a1, nil)
	repos[2].AddBlock(a2, nil)
	repos[2].AddBlock(a3, nil)
	repos[2].AddBlock(a4, nil)
	repos[2].AddBlock(a5, nil)
	repos[2].AddBlock(a6, nil)
	repos[2].AddBlock(b1, nil)
	repos[2].AddBlock(b2, nil)
	repos[2].AddBlock(b3, nil)
	repos[2].AddBlock(b4, nil)
	repos[2].AddBlock(b5, nil)
	repos[2].AddBlock(b6, nil)
	repos[2].AddBlock(c1, nil)
	repos[2].AddBlock(c2, nil)
	repos[2].AddBlock(c3, nil)
	repos[2].AddBlock(c4, nil)
	repos[2].AddBlock(c5, nil)
	repos[2].AddBlock(c6, nil)

	bftCons[2].repo.SetBestBlockID(c6.Header().ID())
	assert.Nil(t, bftCons[2].Update(c6))

	assert.Equal(t, c4.Header().ID(), bftCons[2].state[NV])
	assert.Equal(t, c4.Header().ID(), bftCons[2].state[PP])
	assert.Equal(t, c1.Header().ID(), bftCons[2].state[PC])
}

func TestLiveness1(t *testing.T) {
	initNodesStatus(t)

	// node 0-32
	repos[0].AddBlock(b6, nil)
	repos[0].SetBestBlockID(b6.Header().ID())
	assert.Nil(t, bftCons[0].Update(b6))

	repos[0].AddBlock(c5, nil)
	repos[0].AddBlock(c6, nil)
	repos[0].SetBestBlockID(c6.Header().ID())
	assert.Nil(t, bftCons[0].Update(c6))

	assert.Equal(t, c4.Header().ID(), bftCons[0].state[NV])
	assert.Equal(t, c4.Header().ID(), bftCons[0].state[PP])
	assert.Equal(t, c1.Header().ID(), bftCons[0].state[PC])

	// node 33-65
	repos[1].AddBlock(c6, nil)
	repos[1].SetBestBlockID(c6.Header().ID())
	assert.Nil(t, bftCons[1].Update(c6))

	assert.Equal(t, c4.Header().ID(), bftCons[1].state[NV])
	assert.Equal(t, c4.Header().ID(), bftCons[1].state[PP])
	assert.Equal(t, c1.Header().ID(), bftCons[1].state[PC])
}

func TestLiveness2(t *testing.T) {
	////////////////////
	// Initialization //
	////////////////////
	initNodesStatus(t)

	a7 := newBlock(0, inds[1:33], a6, 1, [4]thor.Bytes32{GenNVForFirstBlock(7), a4.Header().ID(), a1.Header().ID()})
	b7 := newBlock(33, inds[34:45], b6, 1, [4]thor.Bytes32{GenNVForFirstBlock(7), b4.Header().ID(), b1.Header().ID()})
	c7, err := newBlock1(
		66, inds[67:101], c6, c6.Header().Timestamp()+thor.BlockInterval,
		uint64(nNode), nil, repos[2].GenesisBlock().Header().StateRoot(),
		[4]thor.Bytes32{GenNVForFirstBlock(7), c4.Header().ID(), c1.Header().ID()})
	assert.Nil(t, err)

	/////////////////////
	// Synchronization //
	/////////////////////
	update := func(c int, b *block.Block, isBest bool, isSigner bool) {
		repos[c].AddBlock(b, nil)
		if isBest {
			repos[c].SetBestBlockID(b.Header().ID())
		}
		if isSigner {
			assert.Nil(t, bftCons[c].UpdateLastSignedPC(b))
		}
		assert.Nil(t, bftCons[c].Update(b))
	}

	// node 0-32
	update(0, a7, true, true)
	update(0, b6, false, false)
	update(0, b7, true, false)
	update(0, c5, false, false)
	update(0, c6, false, false)
	update(0, c7, true, false)
	assert.Equal(t, c7.Header().ID(), bftCons[0].state[NV])
	assert.Equal(t, emptyID, bftCons[0].state[PP])
	assert.Equal(t, emptyID, bftCons[0].state[PC])

	// node 33-44
	update(1, a7, false, false)
	update(1, b7, true, true)
	update(1, c5, false, false)
	update(1, c6, false, false)
	update(1, c7, true, false)
	assert.Equal(t, c7.Header().ID(), bftCons[1].state[NV])
	assert.Equal(t, emptyID, bftCons[1].state[PP])
	assert.Equal(t, emptyID, bftCons[1].state[PC])

	// node 66-100
	update(2, a7, false, false)
	update(2, b7, false, false)
	update(2, c7, true, true)
	assert.Equal(t, c7.Header().ID(), bftCons[2].state[NV])
	assert.Equal(t, c4.Header().ID(), bftCons[2].state[PP])
	assert.Equal(t, c1.Header().ID(), bftCons[2].state[PC])

	///////////////////////
	// Post-Sync process //
	///////////////////////
	var (
		seeder *poa.Seeder
		stater *state.Stater
		// ncCons *consensus.Consensus
	)
	seeder = poa.NewSeeder(repos[0])
	stater = state.NewStater(dbs[0])
	// ncCons = consensus.New(repos[0], stater, thor.ForkConfig{VIP193: 1, VIP191: math.MaxUint32})

	updateAll := func(b *block.Block) {
		for i := 0; i < 3; i++ {
			repos[i].AddBlock(b, nil)
			repos[i].SetBestBlockID(b.Header().ID())
			assert.Nil(t, bftCons[i].Update(b))
		}
	}

	ts := c7.Header().Timestamp()
	parent := c7
	prevCM := bftCons[0].state[CM]
	for {
		ts = ts + thor.BlockInterval

		st := stater.NewState(parent.Header().StateRoot())
		proposers, err := getProposers(st)
		assert.Nil(t, err)
		proposer, inactives, score := getProposer(seeder, parent, ts, proposers)
		qualified := getCommittee(seeder, parent, proposer, inds)

		// nodes 45-65 stay offline
		if proposer > 44 && proposer < 66 {
			continue
		}

		var backers []int
		for _, q := range qualified {
			if q == proposer || (q > 44 && q < 66) {
				continue
			}
			if (proposer < 45 && q < 45) || (proposer > 65 && q > 65) {
				backers = append(backers, q)
			}
		}

		seed, err := seeder.Generate(parent.Header().ID())
		assert.Nil(t, err)
		updateState(st, backers, inactives)
		stage, _ := st.Stage()
		stateRoot, err := stage.Commit()
		assert.Nil(t, err)

		var fv [4]thor.Bytes32
		if proposer < 45 {
			fv = bftCons[0].state
		} else {
			fv = bftCons[2].state
		}

		c := repos[0].NewChain(parent.Header().ID())
		view, err := newView(c, block.Number(fv[NV]))
		assert.Nil(t, err)
		if view == nil {
			fmt.Println("Debug")
		}
		if view.hasQCForNV() {
			fv[0] = GenNVForFirstBlock(parent.Header().Number() + 1)
		}

		newBlock, err := newBlock1(
			proposer, backers, parent, ts, parent.Header().TotalScore()+score, seed.Bytes(), stateRoot, fv,
		)
		assert.Nil(t, err)

		updateAll(newBlock)

		printRes := func() {
			fmt.Printf("Block: ts = %d, p = %d, b = %d\n", ts, proposer, backers)
			fmt.Printf("\tFV0: [%d, %d, %d, %d]\n",
				block.Number(bftCons[0].state[NV]),
				block.Number(bftCons[0].state[PP]),
				block.Number(bftCons[0].state[PC]),
				block.Number(bftCons[0].state[CM]),
			)
			fmt.Printf("\tFV2: [%d, %d, %d, %d]\n",
				block.Number(bftCons[2].state[NV]),
				block.Number(bftCons[2].state[PP]),
				block.Number(bftCons[2].state[PC]),
				block.Number(bftCons[2].state[CM]),
			)
		}
		printRes()

		if bftCons[0].state[CM] != prevCM {
			assert.Equal(t, bftCons[0].state, bftCons[1].state)
			assert.Equal(t, bftCons[0].state, bftCons[2].state)
			break
		}

		parent = newBlock
	}
}

func TestProposer(t *testing.T) {
	// var proposers []poa.Proposer
	// for i := range inds {
	// 	proposers = append(proposers, poa.Proposer{Address: nodeAddress(i), Active: true})
	// }

	db := muxdb.NewMem()
	stater := state.NewStater(db)
	g := newTestGenesisBuilder()
	b0, _, _, _ := g.Build(stater)
	repo, _ := chain.NewRepository(db, b0)
	seeder := poa.NewSeeder(repo)
	cons := consensus.New(repo, stater, thor.ForkConfig{VIP193: 1, VIP191: math.MaxUint32})

	st := stater.NewState(b0.Header().StateRoot())
	proposers, err := getProposers(st)
	assert.Nil(t, err)

	nInterval := 3
	proposer, inactives, score := getProposer(
		seeder, b0, b0.Header().Timestamp()+uint64(nInterval)*thor.BlockInterval, proposers,
	)
	backers := getCommittee(seeder, b0, proposer, inds)
	seed, err := seeder.Generate(b0.Header().ID())
	assert.Nil(t, err)

	state := stater.NewState(b0.Header().StateRoot())
	updateState(state, backers, inactives)
	stage, _ := state.Stage()
	stage.Commit()

	b1, err := newBlock1(
		proposer, backers, b0,
		b0.Header().Timestamp()+uint64(nInterval)*thor.BlockInterval,
		score, seed.Bytes(), stage.Hash(), [4]thor.Bytes32{})
	assert.Nil(t, err)

	_, _, err = cons.Process(b1, b1.Header().Timestamp())
	assert.Nil(t, err)

}

func TestVRF(t *testing.T) {
	key, _ := crypto.GenerateKey()
	alpha := make([]byte, 36)
	rand.Read(alpha)
	beta1, proof, err := ecvrf.NewSecp256k1Sha256Tai().Prove(key, alpha)
	assert.Nil(t, err)
	beta2, err := ecvrf.NewSecp256k1Sha256Tai().Verify(&key.PublicKey, alpha, proof)
	assert.Nil(t, err)
	assert.Equal(t, beta1, beta2)
}

func getProposers(st *state.State) ([]poa.Proposer, error) {
	authority := builtin.Authority.Native(st)
	var candidates *poa.Candidates
	list, err := authority.AllCandidates()
	if err != nil {
		return nil, err
	}
	candidates = poa.NewCandidates(list)

	proposers, err := candidates.Pick(st)
	if err != nil {
		return nil, err
	}

	return proposers, nil
}

func updateState(state *state.State, backers []int, inactives []poa.Proposer) {
	for _, backer := range backers {
		ok, err := builtin.Authority.Native(state).Update(nodeAddress(backer), true)
		if !ok || err != nil {
			panic("Update error")
		}
	}

	for _, inactive := range inactives {
		ok, err := builtin.Authority.Native(state).Update(inactive.Address, false)
		if !ok || err != nil {
			panic("Update error")
		}
	}
}

func getCommittee(seeder *poa.Seeder, parent *block.Block, proposer int, backers []int) (committee []int) {
	seed, err := seeder.Generate(parent.Header().ID())
	if err != nil {
		panic(err)
	}
	alpha := append(seed[:], parent.Header().ID().Bytes()[:4]...)
	for i := range backers {
		if i == proposer {
			continue
		}

		beta, _, err := ecvrf.NewSecp256k1Sha256Tai().Prove(nodes[i], alpha)
		if err != nil {
			panic(err)
		}
		if lucky := poa.EvaluateVRF(beta); lucky {
			committee = append(committee, i)
		}
	}
	return
}

func getProposer(
	seeder *poa.Seeder,
	parent *block.Block,
	blockTime uint64,
	proposers []poa.Proposer,
) (proposer int, inactives []poa.Proposer, score uint64) {
	// blockTime := parent.Header().Timestamp() + uint64(nInterval)*thor.BlockInterval
	u := 0

	seed, err := seeder.Generate(parent.Header().ID())
	if err != nil {
		panic(err)
	}

	sche, err := poa.NewSchedulerV2(nodeAddress(u), proposers, parent, seed.Bytes())
	if err != nil {
		panic(err)
	}

	ifFound := false
	for i := 0; i < nNode; i++ {
		if sche.IsScheduled(blockTime, nodeAddress(i)) {
			proposer = i
			ifFound = true
			break
		}
	}

	if !ifFound {
		panic("Proposer not found")
	}

	inactives, score = sche.Updates(blockTime)

	return
}
