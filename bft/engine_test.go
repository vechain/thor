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
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

type TestBFT struct {
	engine *BFTEngine
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

	engine, err := NewEngine(repo, db, forkCfg)
	if err != nil {
		return nil, err
	}

	return &TestBFT{
		engine: engine,
		repo:   repo,
		stater: state.NewStater(db),
		fc:     forkCfg,
	}, nil
}

func (test *TestBFT) newBlock(parentSummary *chain.BlockSummary, master genesis.DevAccount, vote block.Vote, conflicts uint32) (*chain.BlockSummary, error) {
	packer := packer.New(test.repo, test.stater, master.Address, &thor.Address{}, test.fc)
	flow, err := packer.Mock(parentSummary, parentSummary.Header.Timestamp()+thor.BlockInterval, parentSummary.Header.GasLimit())
	if err != nil {
		return nil, err
	}

	v := vote
	b, stg, _, err := flow.Pack(master.PrivateKey, conflicts, v)
	if err != nil {
		return nil, err
	}

	if _, err = stg.Commit(); err != nil {
		return nil, err
	}

	_, finalize, err := test.engine.Process(b.Header())
	if err != nil {
		return nil, err
	}

	if err = test.repo.AddBlock(b, nil, conflicts); err != nil {
		return nil, err
	}

	if err = finalize(); err != nil {
		return nil, err
	}

	return test.repo.GetBlockSummary(b.Header().ID())
}

func (test *TestBFT) fastForward(cnt int) error {
	parent := test.repo.BestBlockSummary()
	for i := 1; i <= cnt; i++ {
		priv := devAccounts[(int(parent.Header.Number())+1)%len(devAccounts)]

		var err error
		parent, err = test.newBlock(parent, priv, block.COM, 0)
		if err != nil {
			return err
		}
	}

	return test.repo.SetBestBlockID(parent.Header.ID())
}

func (test *TestBFT) fastForwardWithMinority(cnt int) error {
	parent := test.repo.BestBlockSummary()
	for i := 1; i <= cnt; i++ {
		priv := devAccounts[(int(parent.Header.Number())+1)%(len(devAccounts)/3)]

		var err error
		parent, err = test.newBlock(parent, priv, block.COM, 0)
		if err != nil {
			return err
		}
	}

	return test.repo.SetBestBlockID(parent.Header.ID())
}

func (test *TestBFT) buildBranch(cnt int) (*chain.Chain, error) {
	parent := test.repo.BestBlockSummary()
	for i := 1; i <= cnt; i++ {
		// make a offset to pick a different master
		priv := devAccounts[(int(parent.Header.Number())+1+4)%(len(devAccounts))]

		var err error
		parent, err = test.newBlock(parent, priv, block.COM, 1)
		if err != nil {
			return nil, err
		}
	}
	return test.repo.NewChain(parent.Header.ID()), nil
}

func TestNewEngine(t *testing.T) {
	testBFT, err := newTestBft(defaultFC)
	if err != nil {
		t.Fatal(err)
	}

	genID := testBFT.repo.BestBlockSummary().Header.ID()
	assert.Equal(t, genID, testBFT.engine.Finalized())
}

func TestProcessBlock(t *testing.T) {
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

	summary, err := testBFT.newBlock(testBFT.repo.BestBlockSummary(), master, block.COM, 0)
	if err != nil {
		t.Fatal(err)
	}
	newBest, commit, err := testBFT.engine.Process(summary.Header)

	assert.Nil(t, err)
	assert.True(t, newBest)

	assert.Nil(t, commit())
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

	st, err := testBFT.engine.getState(testBFT.repo.BestBlockSummary().Header.ID(), testBFT.repo.GetBlockHeader)
	if err != nil {
		t.Fatal(err)
	}
	assert.False(t, st.Justified)
	assert.Nil(t, st.CommitAt)
	assert.Equal(t, uint32(0), st.Quality)
	assert.Equal(t, genesisID, testBFT.engine.Finalized())

	for i := 0; i < 3; i++ {
		if err := testBFT.fastForwardWithMinority(thor.CheckpointInterval); err != nil {
			t.Fatal(err)
		}

		st, err := testBFT.engine.getState(testBFT.repo.BestBlockSummary().Header.ID(), testBFT.repo.GetBlockHeader)
		if err != nil {
			t.Fatal(err)
		}
		assert.False(t, st.Justified)
		assert.Nil(t, st.CommitAt)
		assert.Equal(t, uint32(0), st.Quality)
		assert.Equal(t, genesisID, testBFT.engine.Finalized())
	}
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

	blkID, err := testBFT.repo.NewBestChain().GetBlockID(blockNum)
	if err != nil {
		t.Fatal(err)
	}

	st, err := testBFT.engine.getState(blkID, testBFT.repo.GetBlockHeader)
	if err != nil {
		t.Fatal(err)
	}

	// should be justify and commit at (MaxBlockProposers*2/3) + 1
	assert.Equal(t, uint32(1), st.Quality)
	assert.True(t, st.Justified)
	assert.Equal(t, blkID, *st.CommitAt)

	blockNum = uint32(thor.CheckpointInterval*2 + MaxBlockProposers*2/3)

	blkID, err = testBFT.repo.NewBestChain().GetBlockID(blockNum)
	if err != nil {
		t.Fatal(err)
	}

	st, err = testBFT.engine.getState(blkID, testBFT.repo.GetBlockHeader)
	if err != nil {
		t.Fatal(err)
	}

	// should be justify and commit at (bft round start) + (MaxBlockProposers*2/3) + 1
	assert.Equal(t, uint32(3), st.Quality)
	assert.True(t, st.Justified)
	assert.Equal(t, blkID, *st.CommitAt)

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
	err = testBFT.engine.Accepts(testBFT.engine.repo.BestBlockSummary().Header.ID())
	assert.Nil(t, err)

	branchID, err := branch.GetBlockID(thor.CheckpointInterval)
	if err != nil {
		t.Fatal(err)
	}

	// blocks in trunk should be rejected
	err = testBFT.engine.Accepts(branchID)
	assert.Equal(t, errConflictWithFinalized, err)
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

				v, err := testBFT.engine.GetVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, block.WIT, v)
			},
		}, {
			"never justified, vote WIT", func(t *testing.T) {
				testBFT, err := newTestBft(defaultFC)
				if err != nil {
					t.Fatal(err)
				}

				testBFT.fastForwardWithMinority(thor.CheckpointInterval * 3)
				v, err := testBFT.engine.GetVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, block.WIT, v)
			},
		}, {
			"never voted other checkpoint, vote COM", func(t *testing.T) {
				testBFT, err := newTestBft(defaultFC)
				if err != nil {
					t.Fatal(err)
				}

				testBFT.fastForward(thor.CheckpointInterval * 3)
				v, err := testBFT.engine.GetVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, block.COM, v)
			},
		}, {
			"voted other checkpoint but not conflict with recent justified, vote COM", func(t *testing.T) {
				testBFT, err := newTestBft(defaultFC)
				if err != nil {
					t.Fatal(err)
				}

				if err = testBFT.fastForward(thor.CheckpointInterval*3 - 2); err != nil {
					t.Fatal(err)
				}

				genesisID := testBFT.repo.GenesisBlock().Header().ID()
				assert.NotEqual(t, genesisID, testBFT.engine.Finalized())

				if err := testBFT.fastForward(1); err != nil {
					t.Fatal(err)
				}

				branch, err := testBFT.buildBranch(2)
				if err != nil {
					t.Fatal(err)
				}

				if err := testBFT.engine.MarkVoted(branch.HeadID()); err != nil {
					t.Fatal(err)
				}

				if err := testBFT.fastForward(3); err != nil {
					t.Fatal(err)
				}

				trunkCP, err := testBFT.repo.NewBestChain().GetBlockID(thor.CheckpointInterval * 3)
				if err != nil {
					t.Fatal(err)
				}
				if err := testBFT.engine.MarkVoted(trunkCP); err != nil {
					t.Fatal(err)
				}

				// should be 2 checkpoints in voted
				assert.Equal(t, 2, len(testBFT.engine.voted))

				v, err := testBFT.engine.GetVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, block.COM, v)
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

				branch, err := testBFT.buildBranch(2)
				if err != nil {
					t.Fatal(err)
				}

				if err := testBFT.engine.MarkVoted(branch.HeadID()); err != nil {
					t.Fatal(err)
				}

				if err := testBFT.fastForward(8); err != nil {
					t.Fatal(err)
				}

				trunkCP, err := testBFT.repo.NewBestChain().GetBlockID(thor.CheckpointInterval * 3)
				if err != nil {
					t.Fatal(err)
				}
				if err := testBFT.engine.MarkVoted(trunkCP); err != nil {
					t.Fatal(err)
				}

				// should be 2 checkpoints in voted
				assert.Equal(t, 2, len(testBFT.engine.voted))

				v, err := testBFT.engine.GetVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, block.WIT, v)
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

				branch, err := testBFT.buildBranch(8)
				if err != nil {
					t.Fatal(err)
				}

				if err := testBFT.engine.MarkVoted(branch.HeadID()); err != nil {
					t.Fatal(err)
				}

				if err := testBFT.fastForward(8); err != nil {
					t.Fatal(err)
				}

				trunkCP, err := testBFT.repo.NewBestChain().GetBlockID(thor.CheckpointInterval * 3)
				if err != nil {
					t.Fatal(err)
				}
				if err := testBFT.engine.MarkVoted(trunkCP); err != nil {
					t.Fatal(err)
				}

				// should be 2 checkpoints in voted
				assert.Equal(t, 2, len(testBFT.engine.voted))

				v, err := testBFT.engine.GetVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, block.WIT, v)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func TestVoteSet(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			"newVoteSet", func(t *testing.T) {
				fc := defaultFC
				testBft, err := newTestBft(fc)
				if err != nil {
					t.Fatal(err)
				}
				vs, err := newVoteSet(testBft.engine, testBft.repo.BestBlockSummary().Header.ID())
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
				vs, err := newVoteSet(testBft.engine, testBft.repo.BestBlockSummary().Header.ID())
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
				vs, err := newVoteSet(testBft.engine, testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, uint32(thor.CheckpointInterval*2), vs.checkpoint)
				assert.Equal(t, uint64(MaxBlockProposers*2/3), vs.threshold)
				assert.Equal(t, uint32(2), vs.getState().Quality)
				assert.False(t, vs.getState().Justified)
				assert.Nil(t, vs.getState().CommitAt)
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
				vs, err := newVoteSet(testBft.engine, testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				var blkID thor.Bytes32
				for i := 0; i <= MaxBlockProposers*2/3; i++ {
					blkID = RandomBytes32()
					vs.addVote(RandomAddress(), block.COM, blkID)
				}

				st := vs.getState()
				assert.Equal(t, uint32(3), st.Quality)
				assert.Equal(t, *st.CommitAt, blkID)
				assert.True(t, st.Justified)

				// add vote after commits，commit/justify stays the same
				vs.addVote(RandomAddress(), block.COM, RandomBytes32())
				st = vs.getState()
				assert.Equal(t, uint32(3), st.Quality)
				assert.Equal(t, *st.CommitAt, blkID)
				assert.True(t, st.Justified)
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
				vs, err := newVoteSet(testBft.engine, testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				var blkID thor.Bytes32
				for i := 0; i <= MaxBlockProposers*2/3; i++ {
					blkID = RandomBytes32()
					vs.addVote(RandomAddress(), block.WIT, blkID)
				}

				st := vs.getState()
				assert.Equal(t, uint32(3), st.Quality)
				assert.True(t, st.Justified)
				assert.Nil(t, st.CommitAt)
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
				vs, err := newVoteSet(testBft.engine, testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				var blkID thor.Bytes32
				// vote <threshold> times COM
				for i := 0; i < MaxBlockProposers*2/3; i++ {
					blkID = RandomBytes32()
					vs.addVote(RandomAddress(), block.COM, blkID)
				}

				master := RandomAddress()
				// master votes WIT
				blkID = RandomBytes32()
				vs.addVote(master, block.WIT, blkID)

				// justifies but not committed
				st := vs.getState()
				assert.True(t, st.Justified)
				assert.Nil(t, st.CommitAt)

				blkID = RandomBytes32()
				// master votes COM
				vs.addVote(master, block.COM, blkID)

				// should be committed
				st = vs.getState()
				assert.Equal(t, *st.CommitAt, blkID)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}
