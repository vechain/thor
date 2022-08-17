// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"crypto/rand"
	"math"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

type TestBFT struct {
	engine *BFTEngine
	db     *muxdb.MuxDB
	repo   *chain.Repository
	stater *state.Stater
	fc     thor.ForkConfig
}

const MaxBlockProposers = 11

var devAccounts = genesis.DevAccounts()
var defaultFC = thor.ForkConfig{
	VIP191:    math.MaxUint32,
	ETH_CONST: math.MaxUint32,
	BLOCKLIST: math.MaxUint32,
	ETH_IST:   math.MaxUint32,
	VIP214:    math.MaxUint32,
	FINALITY:  0,
}

func RandomAddress() thor.Address {
	var addr thor.Address

	rand.Read(addr[:])
	return addr
}

func RandomBytes32() thor.Bytes32 {
	var b32 thor.Bytes32

	rand.Read(b32[:])
	return b32
}

func newTestBft(forkCfg thor.ForkConfig) (*TestBFT, error) {
	db := muxdb.NewMem()

	auth := make([]genesis.Authority, 0, len(devAccounts))
	for _, acc := range devAccounts {
		auth = append(auth, genesis.Authority{
			MasterAddress:   acc.Address,
			EndorsorAddress: acc.Address,
			Identity:        thor.BytesToBytes32([]byte("master")),
		})
	}
	mbp := uint64(MaxBlockProposers)
	genConfig := genesis.CustomGenesis{
		LaunchTime: 1526400000,
		GasLimit:   thor.InitialGasLimit,
		ExtraData:  "",
		ForkConfig: &forkCfg,
		Authority:  auth,
		Params: genesis.Params{
			MaxBlockProposers: &mbp,
		},
	}

	builder, err := genesis.NewCustomNet(&genConfig)
	if err != nil {
		return nil, err
	}

	stater := state.NewStater(db)
	genesis, _, _, err := builder.Build(stater)
	if err != nil {
		return nil, err
	}

	repo, err := chain.NewRepository(db, genesis)
	if err != nil {
		return nil, err
	}

	engine, err := NewEngine(repo, db, forkCfg, devAccounts[len(devAccounts)-1].Address)
	if err != nil {
		return nil, err
	}

	// touch get vote func to init voted
	_, err = engine.ShouldVote(repo.NewBestChain().GenesisID())
	if err != nil {
		return nil, err
	}

	return &TestBFT{
		engine: engine,
		db:     db,
		repo:   repo,
		stater: state.NewStater(db),
		fc:     forkCfg,
	}, nil
}

func (test *TestBFT) reCreateEngine() error {
	engine, err := NewEngine(test.repo, test.db, test.engine.forkConfig, devAccounts[len(devAccounts)-1].Address)
	if err != nil {
		return err
	}

	// touch get vote func to init voted
	_, err = engine.ShouldVote(test.repo.NewBestChain().GenesisID())
	if err != nil {
		return err
	}

	test.engine = engine
	return nil
}

func (test *TestBFT) newBlock(parentSummary *chain.BlockSummary, master genesis.DevAccount, shouldVote bool) (*chain.BlockSummary, error) {
	packer := packer.New(test.repo, test.stater, master.Address, &thor.Address{}, test.fc)
	flow, err := packer.Mock(parentSummary, parentSummary.Header.Timestamp()+thor.BlockInterval, parentSummary.Header.GasLimit())
	if err != nil {
		return nil, err
	}

	conflicts, err := test.repo.ScanConflicts(parentSummary.Header.Number() + 1)
	if err != nil {
		return nil, err
	}

	b, stg, _, err := flow.Pack(master.PrivateKey, conflicts, shouldVote)
	if err != nil {
		return nil, err
	}

	if _, err = stg.Commit(); err != nil {
		return nil, err
	}

	if err = test.repo.AddBlock(b, nil, conflicts); err != nil {
		return nil, err
	}

	if err = test.engine.CommitBlock(b.Header(), false); err != nil {
		return nil, err
	}

	return test.repo.GetBlockSummary(b.Header().ID())
}

func (test *TestBFT) fastForward(cnt int) error {
	parent := test.repo.BestBlockSummary()

	devCnt := len(devAccounts) - 1
	for i := 1; i <= cnt; i++ {
		acc := devAccounts[(int(parent.Header.Number())+1)%devCnt]

		var err error
		parent, err = test.newBlock(parent, acc, true)
		if err != nil {
			return err
		}
	}

	return test.repo.SetBestBlockID(parent.Header.ID())
}

func (test *TestBFT) fastForwardWithMinority(cnt int) error {
	parent := test.repo.BestBlockSummary()

	devCnt := len(devAccounts) - 1
	for i := 1; i <= cnt; i++ {
		acc := devAccounts[(int(parent.Header.Number())+1)%(devCnt/3)]

		var err error
		parent, err = test.newBlock(parent, acc, true)
		if err != nil {
			return err
		}
	}

	return test.repo.SetBestBlockID(parent.Header.ID())
}

func (test *TestBFT) buildBranch(cnt int) (*chain.Chain, error) {
	parent := test.repo.BestBlockSummary()
	devCnt := len(devAccounts) - 1
	for i := 1; i <= cnt; i++ {
		// make a offset to pick a different master
		acc := devAccounts[(int(parent.Header.Number())+1+4)%devCnt]

		var err error
		parent, err = test.newBlock(parent, acc, true)
		if err != nil {
			return nil, err
		}
	}
	return test.repo.NewChain(parent.Header.ID()), nil
}

func (test *TestBFT) pack(parentID thor.Bytes32, shouldVote bool, best bool) (*chain.BlockSummary, error) {
	acc := devAccounts[len(devAccounts)-1]
	parent, err := test.repo.GetBlockSummary(parentID)
	if err != nil {
		return nil, err
	}

	blk, err := test.newBlock(parent, acc, shouldVote)
	if err != nil {
		return nil, err
	}

	if err := test.engine.CommitBlock(blk.Header, true); err != nil {
		return nil, err
	}

	if best {
		if err := test.repo.SetBestBlockID(blk.Header.ID()); err != nil {
			return nil, err
		}
	}

	return test.repo.GetBlockSummary(blk.Header.ID())
}

func TestNewEngine(t *testing.T) {
	testBFT, err := newTestBft(defaultFC)
	if err != nil {
		t.Fatal(err)
	}

	genID := testBFT.repo.BestBlockSummary().Header.ID()
	assert.Equal(t, genID, testBFT.engine.Finalized())
}

func TestNewBlock(t *testing.T) {
	testBFT, err := newTestBft(defaultFC)
	if err != nil {
		t.Fatal(err)
	}

	if err = testBFT.fastForward(thor.CheckpointInterval - 1); err != nil {
		t.Fatal(err)
	}

	priv, _ := crypto.GenerateKey()

	master := genesis.DevAccount{
		Address:    thor.Address(crypto.PubkeyToAddress(priv.PublicKey)),
		PrivateKey: priv,
	}

	summary, err := testBFT.newBlock(testBFT.repo.BestBlockSummary(), master, true)
	if err != nil {
		t.Fatal(err)
	}

	newBest, err := testBFT.engine.Select(summary.Header)
	assert.Nil(t, err)
	assert.True(t, newBest)

	assert.Nil(t, testBFT.engine.CommitBlock(summary.Header, false))
}

func TestNeverReachJustified(t *testing.T) {
	testBFT, err := newTestBft(defaultFC)
	if err != nil {
		t.Fatal(err)
	}

	genesisID := testBFT.repo.GenesisBlock().Header().ID()
	if err := testBFT.fastForwardWithMinority(thor.CheckpointInterval - 1); err != nil {
		t.Fatal(err)
	}

	st, err := testBFT.engine.computeState(testBFT.repo.BestBlockSummary().Header)
	if err != nil {
		t.Fatal(err)
	}
	assert.False(t, st.Justified)
	assert.False(t, st.Committed)
	assert.Equal(t, uint32(0), st.Quality)
	assert.Equal(t, genesisID, testBFT.engine.Finalized())

	for i := 0; i < 3; i++ {
		if err := testBFT.fastForwardWithMinority(thor.CheckpointInterval); err != nil {
			t.Fatal(err)
		}

		st, err := testBFT.engine.computeState(testBFT.repo.BestBlockSummary().Header)
		if err != nil {
			t.Fatal(err)
		}
		assert.False(t, st.Justified)
		assert.False(t, st.Committed)
		assert.Equal(t, uint32(0), st.Quality)
		assert.Equal(t, genesisID, testBFT.engine.Finalized())
	}
}

func TestReCreate(t *testing.T) {
	testBFT, err := newTestBft(defaultFC)
	if err != nil {
		t.Fatal(err)
	}

	genesisID := testBFT.repo.GenesisBlock().Header().ID()
	if err := testBFT.fastForwardWithMinority(thor.CheckpointInterval - 2); err != nil {
		t.Fatal(err)
	}

	if _, err := testBFT.pack(testBFT.repo.BestBlockSummary().Header.ID(), true, true); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, genesisID, testBFT.engine.Finalized())

	if err := testBFT.fastForwardWithMinority(thor.CheckpointInterval*2 - 1); err != nil {
		t.Fatal(err)
	}

	if _, err := testBFT.pack(testBFT.repo.BestBlockSummary().Header.ID(), true, true); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, genesisID, testBFT.engine.Finalized())

	if err := testBFT.reCreateEngine(); err != nil {
		t.Fatal(err)
	}

	votes := testBFT.engine.casts.Slice(testBFT.engine.Finalized())
	assert.Equal(t, 1, len(votes))
}

func TestFinalized(t *testing.T) {
	testBFT, err := newTestBft(defaultFC)
	if err != nil {
		t.Fatal(err)
	}

	if err = testBFT.fastForward(thor.CheckpointInterval*3 - 1); err != nil {
		t.Fatal(err)
	}

	blockNum := uint32(MaxBlockProposers*2/3 + 1)

	sum, err := testBFT.repo.NewBestChain().GetBlockSummary(blockNum)
	if err != nil {
		t.Fatal(err)
	}

	st, err := testBFT.engine.computeState(sum.Header)
	if err != nil {
		t.Fatal(err)
	}

	// should be justify and commit at (MaxBlockProposers*2/3) + 1
	assert.Equal(t, uint32(1), st.Quality)
	assert.True(t, st.Justified)
	assert.True(t, st.Committed)

	blockNum = uint32(thor.CheckpointInterval*2 + MaxBlockProposers*2/3)

	sum, err = testBFT.repo.NewBestChain().GetBlockSummary(blockNum)
	if err != nil {
		t.Fatal(err)
	}

	st, err = testBFT.engine.computeState(sum.Header)
	if err != nil {
		t.Fatal(err)
	}

	// should be justify and commit at (bft round start) + (MaxBlockProposers*2/3) + 1
	assert.Equal(t, uint32(3), st.Quality)
	assert.True(t, st.Justified)
	assert.True(t, st.Committed)

	// chain stops the end of third bft round,should commit the second checkpoint
	finalized, err := testBFT.repo.NewBestChain().GetBlockID(thor.CheckpointInterval)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, finalized, testBFT.engine.Finalized())
}

func TestAccepts(t *testing.T) {
	testBFT, err := newTestBft(defaultFC)
	if err != nil {
		t.Fatal(err)
	}

	if err = testBFT.fastForward(thor.CheckpointInterval - 1); err != nil {
		t.Fatal(err)
	}

	branch, err := testBFT.buildBranch(1)
	if err != nil {
		t.Fatal(err)
	}

	if err = testBFT.fastForward(thor.CheckpointInterval * 2); err != nil {
		t.Fatal(err)
	}

	// new block in trunk should accept
	ok, err := testBFT.engine.Accepts(testBFT.engine.repo.BestBlockSummary().Header.ID())
	assert.Nil(t, err)
	assert.Equal(t, ok, true)

	branchID, err := branch.GetBlockID(thor.CheckpointInterval)
	if err != nil {
		t.Fatal(err)
	}

	// blocks in trunk should be rejected
	ok, err = testBFT.engine.Accepts(branchID)
	assert.Nil(t, err)
	assert.Equal(t, ok, false)
}

func TestGetVote(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			"early stage, vote WIT", func(t *testing.T) {
				testBFT, err := newTestBft(defaultFC)
				if err != nil {
					t.Fatal(err)
				}

				v, err := testBFT.engine.ShouldVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, false, v)
			},
		}, {
			"never justified, vote WIT", func(t *testing.T) {
				testBFT, err := newTestBft(defaultFC)
				if err != nil {
					t.Fatal(err)
				}

				testBFT.fastForwardWithMinority(thor.CheckpointInterval * 3)
				v, err := testBFT.engine.ShouldVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, false, v)
			},
		}, {
			"never voted other checkpoint, vote COM", func(t *testing.T) {
				testBFT, err := newTestBft(defaultFC)
				if err != nil {
					t.Fatal(err)
				}

				testBFT.fastForward(thor.CheckpointInterval * 3)
				v, err := testBFT.engine.ShouldVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, true, v)
			},
		}, {
			"voted other checkpoint but not conflict with recent justified, vote COM", func(t *testing.T) {
				testBFT, err := newTestBft(defaultFC)
				if err != nil {
					t.Fatal(err)
				}

				if err = testBFT.fastForward(thor.CheckpointInterval*3 - 1); err != nil {
					t.Fatal(err)
				}

				genesisID := testBFT.repo.GenesisBlock().Header().ID()
				assert.NotEqual(t, genesisID, testBFT.engine.Finalized())

				branch, err := testBFT.buildBranch(1)
				if err != nil {
					t.Fatal(err)
				}

				if _, err := testBFT.pack(branch.HeadID(), true, false); err != nil {
					t.Fatal(err)
				}

				if err := testBFT.fastForward(1); err != nil {
					t.Fatal(err)
				}

				if _, err := testBFT.pack(testBFT.repo.BestBlockSummary().Header.ID(), true, true); err != nil {
					t.Fatal(err)
				}

				// should be 2 checkpoints in voted
				votes := testBFT.engine.casts.Slice(testBFT.engine.Finalized())
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, 2, len(votes))

				v, err := testBFT.engine.ShouldVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, true, v)
			},
		}, {
			"voted another non-justified checkpoint,conflict with most recent justified checkpoint, vote WIT", func(t *testing.T) {
				testBFT, err := newTestBft(defaultFC)
				if err != nil {
					t.Fatal(err)
				}

				if err = testBFT.fastForward(thor.CheckpointInterval*3 - 1); err != nil {
					t.Fatal(err)
				}

				genesisID := testBFT.repo.GenesisBlock().Header().ID()
				assert.NotEqual(t, genesisID, testBFT.engine.Finalized())

				branch, err := testBFT.buildBranch(1)
				if err != nil {
					t.Fatal(err)
				}

				if _, err = testBFT.pack(branch.HeadID(), true, false); err != nil {
					t.Fatal(err)
				}

				if err := testBFT.fastForward(7); err != nil {
					t.Fatal(err)
				}

				if _, err := testBFT.pack(testBFT.repo.BestBlockSummary().Header.ID(), true, true); err != nil {
					t.Fatal(err)
				}

				// should be 2 checkpoints in voted
				votes := testBFT.engine.casts.Slice(testBFT.engine.Finalized())
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, 2, len(votes))

				v, err := testBFT.engine.ShouldVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, false, v)

				err = testBFT.reCreateEngine()
				assert.Nil(t, err)

				//should be 2 checkpoints in voted
				votes = testBFT.engine.casts.Slice(testBFT.engine.Finalized())
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, 2, len(votes))

				v, err = testBFT.engine.ShouldVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, false, v)
			},
		}, {
			"voted another justified checkpoint,conflict with most recent justified checkpoint, vote WIT", func(t *testing.T) {
				testBFT, err := newTestBft(defaultFC)
				if err != nil {
					t.Fatal(err)
				}

				if err = testBFT.fastForward(thor.CheckpointInterval*3 - 1); err != nil {
					t.Fatal(err)
				}

				genesisID := testBFT.repo.GenesisBlock().Header().ID()
				assert.NotEqual(t, genesisID, testBFT.engine.Finalized())

				branch, err := testBFT.buildBranch(7)
				if err != nil {
					t.Fatal(err)
				}

				if _, err = testBFT.pack(branch.HeadID(), true, false); err != nil {
					t.Fatal(err)
				}

				if err := testBFT.fastForward(7); err != nil {
					t.Fatal(err)
				}

				if _, err := testBFT.pack(testBFT.repo.BestBlockSummary().Header.ID(), true, true); err != nil {
					t.Fatal(err)
				}

				// should be 2 checkpoints in voted
				votes := testBFT.engine.casts.Slice(testBFT.engine.Finalized())
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, 2, len(votes))

				v, err := testBFT.engine.ShouldVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, false, v)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func TestJustifier(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			"newJustifier", func(t *testing.T) {
				fc := defaultFC
				testBft, err := newTestBft(fc)
				if err != nil {
					t.Fatal(err)
				}
				vs, err := testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, uint32(0), vs.checkpoint)
				assert.Equal(t, uint64(MaxBlockProposers*2/3), vs.threshold)
			},
		}, {
			"fork in the middle of checkpoint", func(t *testing.T) {
				fc := defaultFC
				fc.VIP214 = thor.CheckpointInterval / 2
				testBft, err := newTestBft(fc)
				if err != nil {
					t.Fatal(err)
				}
				vs, err := testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, uint32(0), vs.checkpoint)
				assert.Equal(t, uint64(MaxBlockProposers*2/3), vs.threshold)
			},
		}, {
			"the second bft round", func(t *testing.T) {
				fc := defaultFC
				fc.VIP214 = thor.CheckpointInterval / 2
				testBft, err := newTestBft(fc)
				if err != nil {
					t.Fatal(err)
				}

				testBft.fastForward(thor.CheckpointInterval * 2)
				vs, err := testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, uint32(thor.CheckpointInterval*2), vs.checkpoint)
				assert.Equal(t, uint64(MaxBlockProposers*2/3), vs.threshold)
				assert.Equal(t, uint32(2), vs.Summarize().Quality)
				assert.False(t, vs.Summarize().Justified)
				assert.False(t, vs.Summarize().Committed)
			},
		}, {
			"add votes: commits", func(t *testing.T) {
				fc := defaultFC
				fc.VIP214 = thor.CheckpointInterval / 2
				testBft, err := newTestBft(fc)
				if err != nil {
					t.Fatal(err)
				}

				testBft.fastForward(thor.CheckpointInterval*2 - 1)
				vs, err := testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				var blkID thor.Bytes32
				for i := 0; i <= MaxBlockProposers*2/3; i++ {
					blkID = RandomBytes32()
					vs.AddBlock(blkID, RandomAddress(), true)
				}

				st := vs.Summarize()
				assert.Equal(t, uint32(3), st.Quality)
				assert.True(t, st.Justified)
				assert.True(t, st.Committed)

				// add vote after commits，commit/justify stays the same
				vs.AddBlock(RandomBytes32(), RandomAddress(), true)
				st = vs.Summarize()
				assert.Equal(t, uint32(3), st.Quality)
				assert.True(t, st.Justified)
				assert.True(t, st.Committed)
			},
		}, {
			"add votes: justifies", func(t *testing.T) {
				fc := defaultFC
				fc.VIP214 = thor.CheckpointInterval / 2
				testBft, err := newTestBft(fc)
				if err != nil {
					t.Fatal(err)
				}

				testBft.fastForward(thor.CheckpointInterval*2 - 1)
				vs, err := testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				var blkID thor.Bytes32
				for i := 0; i <= MaxBlockProposers*2/3; i++ {
					blkID = RandomBytes32()
					vs.AddBlock(blkID, RandomAddress(), false)
				}

				st := vs.Summarize()
				assert.Equal(t, uint32(3), st.Quality)
				assert.True(t, st.Justified)
				assert.False(t, st.Committed)
			},
		}, {
			"add votes: one votes WIT then changes to COM", func(t *testing.T) {
				fc := defaultFC
				fc.VIP214 = thor.CheckpointInterval / 2
				testBft, err := newTestBft(fc)
				if err != nil {
					t.Fatal(err)
				}

				testBft.fastForward(thor.CheckpointInterval*2 - 1)
				vs, err := testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				var blkID thor.Bytes32
				// vote <threshold> times COM
				for i := 0; i < MaxBlockProposers*2/3; i++ {
					blkID = RandomBytes32()
					vs.AddBlock(blkID, RandomAddress(), true)
				}

				master := RandomAddress()
				// master votes WIT
				blkID = RandomBytes32()
				vs.AddBlock(blkID, master, false)

				// justifies but not committed
				st := vs.Summarize()
				assert.True(t, st.Justified)
				assert.False(t, st.Committed)

				blkID = RandomBytes32()
				// master votes COM
				vs.AddBlock(blkID, master, true)

				// should not be committed
				st = vs.Summarize()
				assert.False(t, st.Committed)

				// another master votes WIT
				blkID = RandomBytes32()
				vs.AddBlock(blkID, RandomAddress(), true)
				st = vs.Summarize()
				assert.True(t, st.Committed)
			},
		}, {
			"vote both WIT and COM in one round", func(t *testing.T) {
				fc := defaultFC
				testBft, err := newTestBft(fc)
				if err != nil {
					t.Fatal(err)
				}
				vs, err := testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				master := RandomAddress()
				vs.AddBlock(RandomBytes32(), master, true)
				assert.Equal(t, true, vs.votes[master])
				assert.Equal(t, uint64(1), vs.comVotes)

				vs.AddBlock(RandomBytes32(), master, false)
				assert.Equal(t, false, vs.votes[master])
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(RandomBytes32(), master, true)
				assert.Equal(t, false, vs.votes[master])
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(RandomBytes32(), master, false)
				assert.Equal(t, false, vs.votes[master])
				assert.Equal(t, uint64(0), vs.comVotes)

				vs, err = testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}
				vs.AddBlock(RandomBytes32(), master, false)
				assert.Equal(t, false, vs.votes[master])
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(RandomBytes32(), master, true)
				assert.Equal(t, false, vs.votes[master])
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(RandomBytes32(), master, true)
				assert.Equal(t, false, vs.votes[master])
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(RandomBytes32(), master, false)
				assert.Equal(t, false, vs.votes[master])
				assert.Equal(t, uint64(0), vs.comVotes)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}
