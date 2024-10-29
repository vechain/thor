// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
)

type TestBFT struct {
	engine *Engine
	db     *muxdb.MuxDB
	repo   *chain.Repository
	stater *state.Stater
	fc     thor.ForkConfig
}

const MaxBlockProposers = 11

var (
	devAccounts = genesis.DevAccounts()
	defaultFC   = thor.NoFork
)

func init() {
	defaultFC.FINALITY = 0
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

	if b.Header().Number() >= test.fc.FINALITY {
		if err = test.engine.CommitBlock(b.Header(), false); err != nil {
			return nil, err
		}
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

	if blk.Header.Number() >= test.fc.FINALITY {
		if err := test.engine.CommitBlock(blk.Header, true); err != nil {
			return nil, err
		}
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

	j, err := testBFT.engine.Justified()
	assert.Nil(t, err)
	assert.Equal(t, genID, j)
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

	jc, err := testBFT.repo.NewBestChain().GetBlockID(thor.CheckpointInterval * 2)
	if err != nil {
		t.Fatal(err)
	}

	j, err := testBFT.engine.Justified()
	assert.NoError(t, err)
	assert.Equal(t, jc, j)
	assert.Equal(t, jc, testBFT.engine.justified.Load().(justified).value)
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
		}, {
			"test findCheckpointByQuality edge case, should not fail", func(t *testing.T) {
				testBFT, err := newTestBft(defaultFC)
				if err != nil {
					t.Fatal(err)
				}

				testBFT.fastForwardWithMinority(thor.CheckpointInterval * 3)
				testBFT.fastForward(thor.CheckpointInterval*1 + 3)
				_, err = testBFT.engine.ShouldVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}
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

				for i := 0; i <= MaxBlockProposers*2/3; i++ {
					vs.AddBlock(datagen.RandAddress(), true)
				}

				st := vs.Summarize()
				assert.Equal(t, uint32(3), st.Quality)
				assert.True(t, st.Justified)
				assert.True(t, st.Committed)

				// add vote after commits，commit/justify stays the same
				vs.AddBlock(datagen.RandAddress(), true)
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

				for i := 0; i <= MaxBlockProposers*2/3; i++ {
					vs.AddBlock(datagen.RandAddress(), false)
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

				// vote <threshold> times COM
				for i := 0; i < MaxBlockProposers*2/3; i++ {
					vs.AddBlock(datagen.RandAddress(), true)
				}

				master := datagen.RandAddress()
				// master votes WIT
				vs.AddBlock(master, false)

				// justifies but not committed
				st := vs.Summarize()
				assert.True(t, st.Justified)
				assert.False(t, st.Committed)

				// master votes COM
				vs.AddBlock(master, true)

				// should not be committed
				st = vs.Summarize()
				assert.False(t, st.Committed)

				// another master votes WIT
				vs.AddBlock(datagen.RandAddress(), true)
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

				master := datagen.RandAddress()
				vs.AddBlock(master, true)
				assert.Equal(t, true, vs.votes[master])
				assert.Equal(t, uint64(1), vs.comVotes)

				vs.AddBlock(master, false)
				assert.Equal(t, false, vs.votes[master])
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(master, true)
				assert.Equal(t, false, vs.votes[master])
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(master, false)
				assert.Equal(t, false, vs.votes[master])
				assert.Equal(t, uint64(0), vs.comVotes)

				vs, err = testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}
				vs.AddBlock(master, false)
				assert.Equal(t, false, vs.votes[master])
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(master, true)
				assert.Equal(t, false, vs.votes[master])
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(master, true)
				assert.Equal(t, false, vs.votes[master])
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(master, false)
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

func TestJustified(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			"first several rounds, never justified", func(t *testing.T) {
				testBFT, err := newTestBft(defaultFC)
				if err != nil {
					t.Fatal(err)
				}

				for i := 0; i < 3*thor.CheckpointInterval; i++ {
					if err = testBFT.fastForwardWithMinority(1); err != nil {
						t.Fatal(err)
					}

					justified, err := testBFT.engine.Justified()
					assert.Nil(t, err)
					assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), justified)
					assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), testBFT.engine.Finalized())
				}
			},
		}, {
			"first several rounds, get justified", func(t *testing.T) {
				testBFT, err := newTestBft(defaultFC)
				if err != nil {
					t.Fatal(err)
				}

				for i := 0; i < 2*thor.CheckpointInterval-2; i++ {
					if err = testBFT.fastForward(1); err != nil {
						t.Fatal(err)
					}

					justified, err := testBFT.engine.Justified()
					assert.Nil(t, err)
					assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), justified)
					assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), testBFT.engine.Finalized())
				}

				if err = testBFT.fastForward(1); err != nil {
					t.Fatal(err)
				}
				justified, err := testBFT.engine.Justified()
				assert.Nil(t, err)
				assert.Equal(t, uint32(thor.CheckpointInterval), block.Number(justified))
				assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), testBFT.engine.Finalized())
			},
		}, {
			"first three not justified rounds, then justified", func(t *testing.T) {
				testBFT, err := newTestBft(defaultFC)
				if err != nil {
					t.Fatal(err)
				}

				if err = testBFT.fastForwardWithMinority(3*thor.CheckpointInterval - 1); err != nil {
					t.Fatal(err)
				}

				justified, err := testBFT.engine.Justified()
				assert.Nil(t, err)
				assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), justified)
				assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), testBFT.engine.Finalized())

				if err = testBFT.fastForward(thor.CheckpointInterval); err != nil {
					t.Fatal(err)
				}
				justified, err = testBFT.engine.Justified()
				assert.Nil(t, err)
				assert.Equal(t, uint32(3*thor.CheckpointInterval), block.Number(justified))
				assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), testBFT.engine.Finalized())
			},
		}, {
			"get finalized, then justified", func(t *testing.T) {
				testBFT, err := newTestBft(defaultFC)
				if err != nil {
					t.Fatal(err)
				}

				if err = testBFT.fastForward(3*thor.CheckpointInterval - 1); err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, uint32(thor.CheckpointInterval), block.Number(testBFT.engine.Finalized()))

				if err = testBFT.fastForward(thor.CheckpointInterval - 1); err != nil {
					t.Fatal(err)
				}

				justified, err := testBFT.engine.Justified()
				assert.Nil(t, err)
				// current epoch is not concluded
				assert.Equal(t, uint32(2*thor.CheckpointInterval), block.Number(justified))

				if err = testBFT.fastForward(1); err != nil {
					t.Fatal(err)
				}
				justified, err = testBFT.engine.Justified()
				assert.Nil(t, err)
				assert.Equal(t, uint32(3*thor.CheckpointInterval), block.Number(justified))
			},
		}, {
			"get finalized, not justified, then justified", func(t *testing.T) {
				type tJustified = justified
				testBFT, err := newTestBft(defaultFC)
				if err != nil {
					t.Fatal(err)
				}

				if err = testBFT.fastForward(3*thor.CheckpointInterval - 1); err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, uint32(thor.CheckpointInterval), block.Number(testBFT.engine.Finalized()))

				if err = testBFT.fastForwardWithMinority(thor.CheckpointInterval); err != nil {
					t.Fatal(err)
				}
				justified, err := testBFT.engine.Justified()
				assert.Nil(t, err)
				assert.Equal(t, uint32(2*thor.CheckpointInterval), block.Number(justified))

				if err = testBFT.fastForward(thor.CheckpointInterval); err != nil {
					t.Fatal(err)
				}
				justified, err = testBFT.engine.Justified()
				assert.Nil(t, err)
				assert.Equal(t, uint32(4*thor.CheckpointInterval), block.Number(justified))
				// test cache
				assert.Equal(t, justified, testBFT.engine.justified.Load().(tJustified).value)
			},
		}, {
			"fork in the middle, get justified", func(t *testing.T) {
				fc := defaultFC
				fc.FINALITY = thor.CheckpointInterval

				testBFT, err := newTestBft(fc)
				if err != nil {
					t.Fatal(err)
				}

				for i := 0; i < 2*thor.CheckpointInterval-2; i++ {
					if err = testBFT.fastForward(1); err != nil {
						t.Fatal(err)
					}

					justified, err := testBFT.engine.Justified()
					assert.Nil(t, err)
					assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), justified)
					assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), testBFT.engine.Finalized())
				}

				if err = testBFT.fastForward(1); err != nil {
					t.Fatal(err)
				}
				justified, err := testBFT.engine.Justified()
				assert.Nil(t, err)
				assert.Equal(t, uint32(thor.CheckpointInterval), block.Number(justified))
				assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), testBFT.engine.Finalized())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}
