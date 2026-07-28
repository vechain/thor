// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

var (
	validatorStake     = uint64(25_000_000)
	minStakingPeriod   = uint32(360) * 24 * 7
	defaultEpochLength = uint32(180)
)

func init() {
	defaultFC.FINALITY = 0
}

func toWei(vet uint64) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(vet), big.NewInt(1e18))
}

func TestFinalizedPos(t *testing.T) {
	forkCfg := &thor.ForkConfig{
		HAYABUSA: 1,
	}
	hayabusaTP := uint32(1)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

	testBFT, err := newTestBftPos(forkCfg)
	if err != nil {
		t.Fatal(err)
	}

	numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
	if err = testBFT.fastForward(thor.EpochLength()*3 - 1 - numBlksNeededForPos); err != nil {
		t.Fatal(err)
	}

	sum, err := testBFT.repo.NewBestChain().GetBlockSummary(uint32(MaxBlockProposers*2/3 + 1))
	if err != nil {
		t.Fatal(err)
	}

	st, err := testBFT.engine.computeState(sum)
	if err != nil {
		t.Fatal(err)
	}

	// should be justify and commit at firstBlockNum
	assert.Equal(t, uint32(1), st.Quality)
	assert.True(t, st.Justified)
	assert.True(t, st.Committed)

	blockNum := thor.EpochLength()*2 + MaxBlockProposers*2/3

	sum, err = testBFT.repo.NewBestChain().GetBlockSummary(blockNum)
	if err != nil {
		t.Fatal(err)
	}

	st, err = testBFT.engine.computeState(sum)
	if err != nil {
		t.Fatal(err)
	}

	// should be justify and commit at (bft round start) + (MaxBlockProposers*2/3) + 1
	assert.Equal(t, uint32(3), st.Quality)
	assert.True(t, st.Justified)
	assert.True(t, st.Committed)

	// chain stops the end of third bft round,should commit the second checkpoint
	finalized, err := testBFT.repo.NewBestChain().GetBlockID(thor.EpochLength())
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, finalized, testBFT.engine.Finalized())

	jc, err := testBFT.repo.NewBestChain().GetBlockID(thor.EpochLength() * 2)
	if err != nil {
		t.Fatal(err)
	}

	j, err := testBFT.engine.Justified()
	assert.NoError(t, err)
	assert.Equal(t, jc, j)
	assert.Equal(t, jc, testBFT.engine.justified.Load().(justified).value)
}

func TestAcceptsPos(t *testing.T) {
	forkCfg := &thor.ForkConfig{
		HAYABUSA: 1,
	}
	hayabusaTP := uint32(1)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

	testBFT, err := newTestBftPos(forkCfg)
	if err != nil {
		t.Fatal(err)
	}

	numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
	if err = testBFT.fastForward(thor.EpochLength() - 1 - numBlksNeededForPos); err != nil {
		t.Fatal(err)
	}

	branch, err := testBFT.buildBranch(1)
	if err != nil {
		t.Fatal(err)
	}

	if err = testBFT.fastForward(thor.EpochLength() * 2); err != nil {
		t.Fatal(err)
	}

	// new block in trunk should accept
	ok, err := testBFT.engine.Accepts(testBFT.engine.repo.BestBlockSummary().Header.ID())
	assert.Nil(t, err)
	assert.Equal(t, ok, true)

	branchID, err := branch.GetBlockID(thor.EpochLength())
	if err != nil {
		t.Fatal(err)
	}

	// blocks in trunk should be rejected
	ok, err = testBFT.engine.Accepts(branchID)
	assert.Nil(t, err)
	assert.Equal(t, ok, false)
}

func TestGetVotePos(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testing.T, *thor.ForkConfig)
	}{
		{
			"early stage, vote WIT", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBFT, err := newTestBftPos(forkCfg)
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
			"never justified, vote WIT", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBFT, err := newTestBftPos(forkCfg)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
				testBFT.fastForwardWithMinority(thor.EpochLength()*3 - numBlksNeededForPos)
				v, err := testBFT.engine.ShouldVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, false, v)
			},
		}, {
			"never voted other checkpoint, vote COM", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBFT, err := newTestBftPos(forkCfg)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
				testBFT.fastForward(thor.EpochLength()*3 - numBlksNeededForPos)
				v, err := testBFT.engine.ShouldVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, true, v)
			},
		}, {
			"voted other checkpoint but not conflict with recent justified, vote COM", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBFT, err := newTestBftPos(forkCfg)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
				if err = testBFT.fastForward(thor.EpochLength()*3 - 1 - numBlksNeededForPos); err != nil {
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
			"voted another non-justified checkpoint,conflict with most recent justified checkpoint, vote WIT", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBFT, err := newTestBftPos(forkCfg)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
				if err = testBFT.fastForward(thor.EpochLength()*3 - 1 - numBlksNeededForPos); err != nil {
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

				// should be 2 checkpoints in voted
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
			"voted another justified checkpoint,conflict with most recent justified checkpoint, vote WIT", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBFT, err := newTestBftPos(forkCfg)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
				if err = testBFT.fastForward(thor.EpochLength()*3 - 1 - numBlksNeededForPos); err != nil {
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
			"test findCheckpointByQuality edge case, should not fail", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBFT, err := newTestBftPos(forkCfg)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
				testBFT.fastForwardWithMinority(thor.EpochLength()*3 - numBlksNeededForPos)
				testBFT.fastForward(thor.EpochLength()*1 + 3)
				_, err = testBFT.engine.ShouldVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	forkCfg := &thor.ForkConfig{
		HAYABUSA: 1,
	}
	hayabusaTP := uint32(1)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t, forkCfg)
		})
	}
}

func TestJustifierPos(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testing.T, *thor.ForkConfig)
	}{
		{
			"newJustifier", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBft, err := newTestBftPos(forkCfg)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
				testBft.fastForward(thor.EpochLength() - 1 - numBlksNeededForPos)

				vs, err := newJustifierForPending(testBft)
				if err != nil {
					t.Fatal(err)
				}

				if forkCfg.HAYABUSA != thor.NoFork.HAYABUSA {
					assert.Equal(t, uint32(180), vs.checkpoint)
					assert.Equal(t, uint64(166666666), vs.thresholdWeight)
				} else {
					assert.Equal(t, uint32(0), vs.checkpoint)
					assert.Equal(t, uint64(MaxBlockProposers*2/3), vs.thresholdVotes)
				}
			},
		},
		{
			"fork in the middle of checkpoint", func(t *testing.T, forkCfg *thor.ForkConfig) {
				fc := *forkCfg
				fc.VIP214 = thor.EpochLength() / 2
				testBft, err := newTestBftPos(&fc)
				if err != nil {
					t.Fatal(err)
				}
				vs, err := newJustifierForPending(testBft)
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, uint32(0), vs.checkpoint)
				assert.Equal(t, uint64(MaxBlockProposers*2/3), vs.thresholdVotes)
			},
		},
		{
			"the second bft round", func(t *testing.T, forkCfg *thor.ForkConfig) {
				fc := *forkCfg
				fc.VIP214 = thor.EpochLength() / 2
				testBft, err := newTestBftPos(&fc)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
				testBft.fastForward(thor.EpochLength()*2 - numBlksNeededForPos)
				vs, err := newJustifierForPending(testBft)
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, thor.EpochLength()*2, vs.checkpoint)
				assert.Equal(t, uint64(166666666), vs.thresholdWeight)
				assert.Equal(t, uint32(2), vs.Summarize().Quality)
				assert.False(t, vs.Summarize().Justified)
				assert.False(t, vs.Summarize().Committed)
			},
		},
		{
			"add votes: commits", func(t *testing.T, forkCfg *thor.ForkConfig) {
				fc := *forkCfg
				fc.VIP214 = thor.EpochLength() / 2
				testBft, err := newTestBftPos(&fc)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
				testBft.fastForward(thor.EpochLength()*2 - 1 - numBlksNeededForPos)
				vs, err := newJustifierForPending(testBft)
				if err != nil {
					t.Fatal(err)
				}

				for i := 0; i <= MaxBlockProposers*2/3; i++ {
					// Weight, stake multiplied by default multiplier
					vs.AddBlock(datagen.RandAddress(), true, validatorStake*2)
				}

				st := vs.Summarize()
				assert.Equal(t, uint32(3), st.Quality)
				assert.True(t, st.Justified)
				assert.True(t, st.Committed)

				// add vote after commits，commit/justify stays the same
				vs.AddBlock(datagen.RandAddress(), true, validatorStake*2)

				st = vs.Summarize()
				assert.Equal(t, uint32(3), st.Quality)
				assert.True(t, st.Justified)
				assert.True(t, st.Committed)
			},
		},
		{
			"add votes: justifies", func(t *testing.T, forkCfg *thor.ForkConfig) {
				fc := *forkCfg
				fc.VIP214 = thor.EpochLength() / 2
				testBft, err := newTestBftPos(&fc)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
				testBft.fastForward(thor.EpochLength()*2 - 1 - numBlksNeededForPos)
				vs, err := newJustifierForPending(testBft)
				if err != nil {
					t.Fatal(err)
				}

				for i := 0; i <= MaxBlockProposers*2/3; i++ {
					// Weight, stake multiplied by default multiplier
					vs.AddBlock(datagen.RandAddress(), false, validatorStake*2)
				}

				st := vs.Summarize()
				assert.Equal(t, uint32(3), st.Quality)
				assert.True(t, st.Justified)
				assert.False(t, st.Committed)
			},
		},
		{
			"add votes: one votes WIT then changes to COM", func(t *testing.T, forkCfg *thor.ForkConfig) {
				fc := *forkCfg
				fc.VIP214 = thor.EpochLength() / 2
				testBft, err := newTestBftPos(&fc)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
				testBft.fastForward(thor.EpochLength()*2 - 1 - numBlksNeededForPos)
				vs, err := newJustifierForPending(testBft)
				if err != nil {
					t.Fatal(err)
				}

				// vote <threshold> times COM
				// With the current values, weight threshold is 2/3 of 25 * 2 (weight of each validator) * 10 validators = 333.33
				// 2/3 of MaxBlockProposers is 7, so 7 * 25 * 2 * 10 = 350
				// Since 350 > 333.33, committed would be true, hence the - 1 so it is false
				// In PoA we do 2/3 of MaxBlockProposers that rounded is 7, 7 > 7 is false hence committed would be false
				for range MaxBlockProposers*2/3 - 1 {
					// Weight, stake multiplied by default multiplier
					vs.AddBlock(datagen.RandAddress(), true, validatorStake)
				}

				master := datagen.RandAddress()
				// master votes WIT
				if forkCfg.HAYABUSA != thor.NoFork.HAYABUSA {
					vs.AddBlock(master, false, validatorStake)
				} else {
					vs.AddBlock(master, false, 0)
				}

				// justifies but not committed
				st := vs.Summarize()
				assert.True(t, st.Justified)
				assert.False(t, st.Committed)

				// master votes COM
				if forkCfg.HAYABUSA != thor.NoFork.HAYABUSA {
					vs.AddBlock(master, true, validatorStake*2)
				} else {
					vs.AddBlock(master, true, 0)
				}

				// should not be committed
				st = vs.Summarize()
				assert.False(t, st.Committed)

				// another master votes WIT
				if forkCfg.HAYABUSA != thor.NoFork.HAYABUSA {
					vs.AddBlock(datagen.RandAddress(), true, validatorStake*2)
				} else {
					vs.AddBlock(datagen.RandAddress(), true, 0)
				}
				st = vs.Summarize()
				assert.True(t, st.Committed)
			},
		},
		{
			"vote both WIT and COM in one round", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBft, err := newTestBftPos(forkCfg)
				if err != nil {
					t.Fatal(err)
				}
				vs, err := newJustifierForPending(testBft)
				if err != nil {
					t.Fatal(err)
				}

				master := datagen.RandAddress()
				vs.AddBlock(master, true, validatorStake*2)
				assert.Equal(t, true, vs.votes[master].isCOM)
				assert.Equal(t, uint64(1), vs.comVotes)

				vs.AddBlock(master, false, validatorStake*2)
				assert.Equal(t, false, vs.votes[master].isCOM)
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(master, true, validatorStake*2)

				assert.Equal(t, false, vs.votes[master].isCOM)
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(master, false, validatorStake*2)

				assert.Equal(t, false, vs.votes[master].isCOM)
				assert.Equal(t, uint64(0), vs.comVotes)

				vs, err = newJustifierForPending(testBft)
				if err != nil {
					t.Fatal(err)
				}
				vs.AddBlock(master, false, validatorStake*2)

				assert.Equal(t, false, vs.votes[master].isCOM)
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(master, true, validatorStake*2)

				assert.Equal(t, false, vs.votes[master].isCOM)
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(master, true, validatorStake*2)

				assert.Equal(t, false, vs.votes[master].isCOM)
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(master, false, validatorStake*2)

				assert.Equal(t, false, vs.votes[master].isCOM)
				assert.Equal(t, uint64(0), vs.comVotes)
			},
		},
	}

	forkCfg := &thor.ForkConfig{
		HAYABUSA: 1,
	}
	hayabusaTP := uint32(1)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t, forkCfg)
		})
	}
}

func TestJustifiedPos(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(*testing.T, *thor.ForkConfig)
	}{
		{
			"first several rounds, never justified", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBFT, err := newTestBftPos(forkCfg)
				if err != nil {
					t.Fatal(err)
				}

				for range 3 * thor.EpochLength() {
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
			"first several rounds, get justified", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBFT, err := newTestBftPos(forkCfg)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
				if err = testBFT.fastForward(thor.EpochLength() - numBlksNeededForPos); err != nil {
					t.Fatal(err)
				}

				for range thor.EpochLength() - 2 {
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
				assert.Equal(t, thor.EpochLength(), block.Number(justified))
				assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), testBFT.engine.Finalized())
			},
		}, {
			"first three not justified rounds, then justified", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBFT, err := newTestBftPos(forkCfg)
				if err != nil {
					t.Fatal(err)
				}

				if err = testBFT.fastForwardWithMinority(3*thor.EpochLength() - 1); err != nil {
					t.Fatal(err)
				}

				justified, err := testBFT.engine.Justified()
				assert.Nil(t, err)
				assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), justified)
				assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), testBFT.engine.Finalized())

				if err = testBFT.fastForward(thor.EpochLength()); err != nil {
					t.Fatal(err)
				}
				justified, err = testBFT.engine.Justified()
				assert.Nil(t, err)
				assert.Equal(t, 3*thor.EpochLength(), block.Number(justified))
				assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), testBFT.engine.Finalized())
			},
		}, {
			"get finalized, then justified", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBFT, err := newTestBftPos(forkCfg)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1

				if err = testBFT.fastForward(3*thor.EpochLength() - 1 - numBlksNeededForPos); err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, thor.EpochLength(), block.Number(testBFT.engine.Finalized()))

				if err = testBFT.fastForward(thor.EpochLength() - 1); err != nil {
					t.Fatal(err)
				}

				justified, err := testBFT.engine.Justified()
				assert.Nil(t, err)
				// current epoch is not concluded
				assert.Equal(t, 2*thor.EpochLength(), block.Number(justified))

				if err = testBFT.fastForward(1); err != nil {
					t.Fatal(err)
				}
				justified, err = testBFT.engine.Justified()
				assert.Nil(t, err)
				assert.Equal(t, 3*thor.EpochLength(), block.Number(justified))
			},
		}, {
			"get finalized, not justified, then justified", func(t *testing.T, forkCfg *thor.ForkConfig) {
				type tJustified = justified
				testBFT, err := newTestBftPos(forkCfg)
				if err != nil {
					t.Fatal(err)
				}

				if err = testBFT.fastForward(3*thor.EpochLength() - 1); err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, thor.EpochLength(), block.Number(testBFT.engine.Finalized()))

				if err = testBFT.fastForwardWithMinority(thor.EpochLength()); err != nil {
					t.Fatal(err)
				}
				justified, err := testBFT.engine.Justified()
				assert.Nil(t, err)
				assert.Equal(t, 2*thor.EpochLength(), block.Number(justified))

				if err = testBFT.fastForward(thor.EpochLength()); err != nil {
					t.Fatal(err)
				}
				justified, err = testBFT.engine.Justified()
				assert.Nil(t, err)
				assert.Equal(t, 4*thor.EpochLength(), block.Number(justified))
				// test cache
				assert.Equal(t, justified, testBFT.engine.justified.Load().(tJustified).value)
			},
		}, {
			"fork in the middle, get justified", func(t *testing.T, forkCfg *thor.ForkConfig) {
				fc := *forkCfg
				fc.FINALITY = thor.EpochLength()

				testBFT, err := newTestBftPos(&fc)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
				if err = testBFT.fastForward(thor.EpochLength() - numBlksNeededForPos); err != nil {
					t.Fatal(err)
				}

				for range thor.EpochLength() - 2 {
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
				assert.Equal(t, thor.EpochLength(), block.Number(justified))
				assert.Equal(t, testBFT.repo.GenesisBlock().Header().ID(), testBFT.engine.Finalized())
			},
		},
	}

	forkCfg := &thor.ForkConfig{
		HAYABUSA: 1,
	}
	hayabusaTP := uint32(1)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t, forkCfg)
		})
	}
}

func newTestBftPos(forkCfg *thor.ForkConfig) (*TestBFT, error) {
	testBFT, err := newTestBft(forkCfg)
	if err != nil {
		return nil, err
	}

	if err = testBFT.fastForward(forkCfg.HAYABUSA + thor.HayabusaTP()); err != nil {
		return nil, err
	}

	if _, err = testBFT.transitionToPosBlock(testBFT.repo.BestBlockSummary(), devAccounts[0], true, true); err != nil {
		return nil, err
	}

	return testBFT, nil
}

func (test *TestBFT) transitionToPosBlock(parentSummary *chain.BlockSummary, master genesis.DevAccount, shouldVote bool, asBest bool) (
	*chain.BlockSummary, error,
) {
	packer := packer.New(test.repo, test.stater, master.Address, &thor.Address{}, test.fc, 0)
	thor.SetConfig(thor.Config{
		EpochLength: 1,
	})
	flow, _, err := packer.Mock(parentSummary, parentSummary.Header.Timestamp()+thor.BlockInterval(), parentSummary.Header.GasLimit())
	if err != nil {
		return nil, err
	}
	thor.SetConfig(thor.Config{
		EpochLength: defaultEpochLength,
	})

	conflicts, err := test.repo.ScanConflicts(parentSummary.Header.Number() + 1)
	if err != nil {
		return nil, err
	}

	staker := builtin.Staker.Native(test.stater.NewState(parentSummary.Root()))
	validation, err := staker.GetValidation(master.Address)
	if err != nil {
		return nil, err
	}
	if validation == nil {
		// Add all dev accounts as validators
		for _, dev := range devAccounts {
			if err := test.adoptStakerTx(flow, dev.PrivateKey, "addValidation", toWei(validatorStake), dev.Address, minStakingPeriod); err != nil {
				return nil, err
			}
		}
	}

	b, stg, receipts, err := flow.Pack(master.PrivateKey, conflicts, shouldVote)
	if err != nil {
		return nil, err
	}

	if _, err = stg.Commit(); err != nil {
		return nil, err
	}

	if err = test.repo.AddBlock(b, receipts, conflicts, asBest); err != nil {
		return nil, err
	}

	if thor.IsForked(b.Header().Number(), test.fc.FINALITY) {
		if err = test.engine.CommitBlock(b.Header(), conflicts, false); err != nil {
			return nil, err
		}
	}

	return test.repo.GetBlockSummary(b.Header().ID())
}

func (test *TestBFT) adoptStakerTx(flow *packer.Flow, privateKey *ecdsa.PrivateKey, methodName string, value *big.Int, args ...any) error {
	methodABI, found := builtin.Staker.ABI.MethodByName(methodName)
	if !found {
		return fmt.Errorf("%s method not found", methodName)
	}
	data, err := methodABI.EncodeInput(args...)
	if err != nil {
		return err
	}

	clause := tx.NewClause(&builtin.Staker.Address)
	clause = clause.WithValue(value)
	clause = clause.WithData(data)

	trx := new(tx.Builder).
		ChainTag(test.repo.ChainTag()).
		BlockRef(tx.NewBlockRef(test.repo.BestBlockSummary().Header.Number())).
		Expiration(32).
		Nonce(datagen.RandUint64()).
		Gas(1000000).
		Clause(clause).Build()

	signature, err := crypto.Sign(trx.SigningHash().Bytes(), privateKey)
	if err != nil {
		return err
	}
	trx = trx.WithSignature(signature)

	err = flow.Adopt(trx)
	if err != nil {
		return err
	}

	return nil
}

// TestResyncWithAdvancedFinalized: node restarts with finalized ahead of the first
// PoS storePoint. Resync's early iterations must not feed findCheckpointByQuality a
// headID before finalized, or the uint32 range underflows and the lookup fails.
func TestResyncWithAdvancedFinalized(t *testing.T) {
	forkCfg := &thor.ForkConfig{
		HAYABUSA: 1,
	}
	hayabusaTP := uint32(1)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

	testBFT, err := newTestBftPos(forkCfg)
	if err != nil {
		t.Fatal(err)
	}

	numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
	if err = testBFT.fastForward(thor.EpochLength()*3 - 1 - numBlksNeededForPos); err != nil {
		t.Fatal(err)
	}

	// fastForward advanced finalized past firstPosBlock's storePoint — required for
	// this regression to bite.
	firstPosStorePoint := getStorePoint(forkCfg.HAYABUSA + thor.HayabusaTP())
	finalizedBefore := testBFT.engine.Finalized()
	assert.Greater(t, block.Number(finalizedBefore), firstPosStorePoint,
		"test setup precondition: finalized must be past firstPosBlock storePoint")

	// Reset the resync version so Resync() actually runs on this engine.
	if err := testBFT.engine.data.Delete(resyncVersionKey); err != nil {
		t.Fatal(err)
	}

	if err := testBFT.engine.Resync(nil); err != nil {
		t.Fatalf("Resync returned error: %v", err)
	}

	// Resync only advances finalized forward, so it must not have regressed.
	finalizedAfter := testBFT.engine.Finalized()
	assert.GreaterOrEqual(t, block.Number(finalizedAfter), block.Number(finalizedBefore))
}

// TestPosThresholdReadsPostHousekeepState pins that newJustifier reads totalWeight
// from the checkpoint (post-housekeep), not checkpoint-1 — the inverse of the pre-fix
// off-sync where threshold used W_old while votes summed W_old + W_new.
//
// It asserts structural properties (threshold == getTotalWeight(checkpoint)*2/3, PoS
// branch selected), catching formula regressions and PoS/PoA branch swaps. The stock
// test chain activates queued validators within 1 block, so no W-difference exists
// across the checkpoint; the negative assertion against pre-checkpoint weight is thus
// conditional. See review doc §7.2.
func TestPosThresholdReadsPostHousekeepState(t *testing.T) {
	forkCfg := &thor.ForkConfig{
		HAYABUSA: 1,
	}
	hayabusaTP := uint32(1)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

	testBFT, err := newTestBftPos(forkCfg)
	if err != nil {
		t.Fatal(err)
	}

	// Advance just past the first long-epoch checkpoint so it and its predecessor are in repo.
	numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
	if err = testBFT.fastForward(thor.EpochLength() + 1 - numBlksNeededForPos); err != nil {
		t.Fatal(err)
	}

	checkpointID, err := testBFT.repo.NewBestChain().GetBlockID(thor.EpochLength())
	if err != nil {
		t.Fatal(err)
	}
	checkpointSum, err := testBFT.repo.GetBlockSummary(checkpointID)
	if err != nil {
		t.Fatal(err)
	}
	preCheckpointID, err := testBFT.repo.NewBestChain().GetBlockID(thor.EpochLength() - 1)
	if err != nil {
		t.Fatal(err)
	}
	preCheckpointSum, err := testBFT.repo.GetBlockSummary(preCheckpointID)
	if err != nil {
		t.Fatal(err)
	}

	postWeight, err := testBFT.engine.getTotalWeight(checkpointSum)
	if err != nil {
		t.Fatalf("getTotalWeight at checkpoint: %v", err)
	}

	js, err := testBFT.engine.newJustifier(checkpointSum)
	if err != nil {
		t.Fatalf("newJustifier: %v", err)
	}

	// Structural pin: thresholdWeight == postWeight*2/3 and PoS branch selected,
	// regardless of W difference.
	assert.Equal(t, postWeight*2/3, js.thresholdWeight,
		"thresholdWeight must come from post-housekeep state at checkpoint")
	assert.Equal(t, uint64(0), js.thresholdVotes,
		"PoS path must be selected; nonzero thresholdVotes means PoA was taken from pre-housekeep state")

	// Stronger negative pin: only bites when housekeep changed W across the checkpoint.
	// The stock chain doesn't, but we attempt it so future chains that do catch regressions.
	preWeight, preErr := testBFT.engine.getTotalWeight(preCheckpointSum)
	switch {
	case preErr != nil:
		t.Logf("preCheckpoint getTotalWeight returned %v; PoS-branch divergence still pins the contract", preErr)
	case preWeight == postWeight:
		t.Logf("WARN: pre==post==%d; W-difference negative pin not exercised (see test docstring)", preWeight)
	default:
		assert.NotEqual(t, preWeight*2/3, js.thresholdWeight,
			"thresholdWeight equals pre-housekeep value — regression in checkpoint state sourcing")
	}
}

// TestComputeStateCheckpointPlusOneBuildVsReuseEquivalence pins that at checkpoint+1,
// reusing the cached checkpoint justifier + one vote equals building fresh and walking
// back through the checkpoint. Written to check whether the (now-removed)
// !isCheckPoint(parent) guard was needed; it wasn't. Kept as a regression guard against
// newJustifier/AddBlock/Summarize changes that would break the equivalence.
func TestComputeStateCheckpointPlusOneBuildVsReuseEquivalence(t *testing.T) {
	forkCfg := &thor.ForkConfig{
		HAYABUSA: 1,
	}
	hayabusaTP := uint32(1)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

	testBFT, err := newTestBftPos(forkCfg)
	if err != nil {
		t.Fatal(err)
	}

	// Advance past the first PoS checkpoint so checkpoint and checkpoint+1 are in repo.
	numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
	if err = testBFT.fastForward(thor.EpochLength() + 5 - numBlksNeededForPos); err != nil {
		t.Fatal(err)
	}

	checkpointID, err := testBFT.repo.NewBestChain().GetBlockID(thor.EpochLength())
	if err != nil {
		t.Fatal(err)
	}
	checkpointSum, err := testBFT.repo.GetBlockSummary(checkpointID)
	if err != nil {
		t.Fatal(err)
	}
	plusOneID, err := testBFT.repo.NewBestChain().GetBlockID(thor.EpochLength() + 1)
	if err != nil {
		t.Fatal(err)
	}
	plusOneSum, err := testBFT.repo.GetBlockSummary(plusOneID)
	if err != nil {
		t.Fatal(err)
	}

	// Path A (real): computeState at checkpoint fills the cache; at checkpoint+1 it
	// reuses that justifier and appends one vote.
	testBFT.engine.caches.state.Purge()
	testBFT.engine.caches.justifier = cache.NewPrioCache(16)

	if _, err := testBFT.engine.computeState(checkpointSum); err != nil {
		t.Fatalf("computeState(checkpoint): %v", err)
	}
	stateReal, err := testBFT.engine.computeState(plusOneSum)
	if err != nil {
		t.Fatalf("computeState(checkpoint+1): %v", err)
	}

	// Path B (manual): reset caches, process checkpoint, then apply only checkpoint+1's
	// vote to its cached justifier — what the loop with end=header.Number() does.
	testBFT.engine.caches.state.Purge()
	testBFT.engine.caches.justifier = cache.NewPrioCache(16)

	if _, err := testBFT.engine.computeState(checkpointSum); err != nil {
		t.Fatalf("re-computeState(checkpoint): %v", err)
	}
	entry := testBFT.engine.caches.justifier.Remove(checkpointSum.Header.ID())
	if entry == nil {
		t.Fatal("expected checkpoint justifier in cache")
	}
	reusedJs := entry.Value.(*justifier)

	// Apply checkpoint+1's vote, mirroring the loop's AddBlock with end=header.Number().
	signer, _ := plusOneSum.Header.Signer()
	parentSum, err := testBFT.repo.GetBlockSummary(plusOneSum.Header.ParentID())
	if err != nil {
		t.Fatal(err)
	}
	state := testBFT.engine.stater.NewState(parentSum.Root())
	staker := builtin.Staker.Native(state)
	posActive, _ := staker.IsPoSActive()
	var weight uint64
	if posActive {
		val, err := staker.GetValidation(signer)
		if err != nil {
			t.Fatal(err)
		}
		if val == nil {
			t.Fatal("validator not found")
		}
		weight = val.Weight
	}
	reusedJs.AddBlock(signer, plusOneSum.Header.COM(), weight)
	stateManual := reusedJs.Summarize()

	// Contract: both paths must produce identical bftState. Divergence means
	// newJustifier/AddBlock/Summarize broke the "append one vote == loop back" equivalence.
	assert.Equal(t, *stateReal, *stateManual,
		"computeState(checkpoint+1) diverges from manual reproduction; "+
			"vote / threshold / quality semantics changed")
}

// TestFindCheckpointByQualityRejectsHeadBeforeFinalized pins that
// findCheckpointByQuality rejects a (headID, finalized) pair with headID before
// finalized, instead of underflowing the uint32 range. Resync's caller-side epoch
// guard prevents this in normal flow, so this micro-test calls the function directly.
func TestFindCheckpointByQualityRejectsHeadBeforeFinalized(t *testing.T) {
	forkCfg := &thor.ForkConfig{
		HAYABUSA: 1,
	}
	hayabusaTP := uint32(1)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

	testBFT, err := newTestBftPos(forkCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = testBFT.fastForward(thor.EpochLength() * 2); err != nil {
		t.Fatal(err)
	}

	finalizedID, err := testBFT.repo.NewBestChain().GetBlockID(thor.EpochLength())
	if err != nil {
		t.Fatal(err)
	}
	headID, err := testBFT.repo.NewBestChain().GetBlockID(thor.EpochLength() - 1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = testBFT.engine.findCheckpointByQuality(1, finalizedID, headID)
	assert.Error(t, err, "headID precedes finalized must be rejected")
	assert.Contains(t, err.Error(), "headID precedes finalized")
}

// packStakerTxBlock packs, commits and registers one block signed by p carrying a
// single staker-contract tx.
func packStakerTxBlock(t *testing.T, test *TestBFT, p genesis.DevAccount, method string, value *big.Int, args ...any) {
	t.Helper()

	parent := test.repo.BestBlockSummary()
	pk := packer.New(test.repo, test.stater, p.Address, &thor.Address{}, test.fc, 0)
	flow, _, err := pk.Mock(parent, parent.Header.Timestamp()+thor.BlockInterval(), parent.Header.GasLimit())
	if err != nil {
		t.Fatal(err)
	}
	if err := test.adoptStakerTx(flow, p.PrivateKey, method, value, args...); err != nil {
		t.Fatal(err)
	}
	conflicts, err := test.repo.ScanConflicts(parent.Header.Number() + 1)
	if err != nil {
		t.Fatal(err)
	}
	b, stg, receipts, err := flow.Pack(p.PrivateKey, conflicts, true)
	if err != nil {
		t.Fatal(err)
	}
	require.Len(t, receipts, 1)
	require.False(t, receipts[0].Reverted, "staker tx %s reverted", method)
	if _, err := stg.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := test.repo.AddBlock(b, receipts, conflicts, true); err != nil {
		t.Fatal(err)
	}
	if err := test.engine.CommitBlock(b.Header(), conflicts, false); err != nil {
		t.Fatal(err)
	}
}

// TestCheckpointBlockOwnVoteUsesPostHousekeepWeight pins that when the checkpoint
// block's own signer's weight changes at that block's Housekeep, its vote is weighed
// by the post-housekeep weight — the same basis as thresholdWeight — not the stale
// weight from the checkpoint's parent state. The decrease case is the dangerous
// direction: phantom weight inflates comWeight against a threshold that no longer
// includes it.
func TestCheckpointBlockOwnVoteUsesPostHousekeepWeight(t *testing.T) {
	forkCfg := &thor.ForkConfig{
		HAYABUSA: 1,
	}
	hayabusaTP := uint32(1)

	// The harness fastForward forces EpochLength=1 per block (addBlock's
	// quickTransition), so validators activate at startBlock and every block is a
	// potential renewal point, gated only by IsPeriodEnd.
	startBlock := forkCfg.HAYABUSA + hayabusaTP + 2
	checkpoint := thor.EpochLength()

	// Renewal points are StartBlock + k*period. Halving the distance to the
	// checkpoint yields one intermediate renewal point; a change parked after it can
	// only be applied at the checkpoint itself.
	period := (checkpoint - startBlock) / 2
	require.Equal(t, startBlock+2*period, checkpoint, "period must align renewals to the checkpoint")
	intermediateRenewal := startBlock + period

	cases := []struct {
		name         string
		expectedPre  uint64 // P's weight at checkpoint-1 (pre-housekeep)
		expectedPost uint64 // P's weight at checkpoint (post-housekeep)
		prepare      func(t *testing.T, test *TestBFT, p genesis.DevAccount)
	}{
		{
			name:         "increase locked at checkpoint housekeep",
			expectedPre:  validatorStake,
			expectedPost: validatorStake * 2,
			prepare: func(t *testing.T, test *TestBFT, p genesis.DevAccount) {
				// Increase sent past the intermediate renewal locks at the checkpoint.
				best := test.repo.BestBlockSummary().Header.Number()
				require.NoError(t, test.fastForward(intermediateRenewal+1-best))
				packStakerTxBlock(t, test, p, "increaseStake", toWei(validatorStake), p.Address)
			},
		},
		{
			name:         "decrease (PendingUnlockVET) applied at checkpoint housekeep",
			expectedPre:  validatorStake * 2,
			expectedPost: validatorStake,
			prepare: func(t *testing.T, test *TestBFT, p genesis.DevAccount) {
				// Lock an increase at the intermediate renewal first (stake must stay
				// above the minimum), then park a decrease for the checkpoint renewal.
				packStakerTxBlock(t, test, p, "increaseStake", toWei(validatorStake), p.Address)
				best := test.repo.BestBlockSummary().Header.Number()
				require.NoError(t, test.fastForward(intermediateRenewal+1-best))
				packStakerTxBlock(t, test, p, "decreaseStake", big.NewInt(0), p.Address, toWei(validatorStake))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			origMinStakingPeriod := minStakingPeriod
			origLow := thor.LowStakingPeriod()
			minStakingPeriod = period
			thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP, LowStakingPeriod: period})
			defer func() {
				minStakingPeriod = origMinStakingPeriod
				thor.SetConfig(thor.Config{LowStakingPeriod: origLow})
			}()

			testBFT, err := newTestBftPos(forkCfg)
			if err != nil {
				t.Fatal(err)
			}

			// fastForward's round-robin makes devAccounts[0] sign block `checkpoint`.
			p := devAccounts[0]
			require.Zero(t, checkpoint%uint32(len(devAccounts)-1))

			require.NoError(t, testBFT.fastForward(startBlock-testBFT.repo.BestBlockSummary().Header.Number()))

			val, err := builtin.Staker.Native(testBFT.stater.NewState(testBFT.repo.BestBlockSummary().Root())).GetValidation(p.Address)
			if err != nil {
				t.Fatal(err)
			}
			require.NotNil(t, val)
			require.Equal(t, startBlock, val.StartBlock, "harness assumption: validator StartBlock")
			require.Equal(t, period, val.Period, "harness assumption: validator Period")

			tc.prepare(t, testBFT, p)

			require.NoError(t, testBFT.fastForward(checkpoint-testBFT.repo.BestBlockSummary().Header.Number()))

			bestChain := testBFT.repo.NewBestChain()
			checkpointSum, err := bestChain.GetBlockSummary(checkpoint)
			if err != nil {
				t.Fatal(err)
			}
			signer, err := checkpointSum.Header.Signer()
			if err != nil {
				t.Fatal(err)
			}
			require.Equal(t, p.Address, signer, "harness assumption: P signs the checkpoint block")

			preSum, err := bestChain.GetBlockSummary(checkpoint - 1)
			if err != nil {
				t.Fatal(err)
			}
			preVal, err := builtin.Staker.Native(testBFT.stater.NewState(preSum.Root())).GetValidation(p.Address)
			if err != nil {
				t.Fatal(err)
			}
			require.Equal(t, tc.expectedPre, preVal.Weight, "pre-housekeep weight")

			postVal, err := builtin.Staker.Native(testBFT.stater.NewState(checkpointSum.Root())).GetValidation(p.Address)
			if err != nil {
				t.Fatal(err)
			}
			require.Equal(t, tc.expectedPost, postVal.Weight, "post-housekeep weight")

			// Fresh justifier for the checkpoint block (as bft.Select builds it).
			testBFT.engine.caches.state.Purge()
			testBFT.engine.caches.justifier = cache.NewPrioCache(16)
			if _, err := testBFT.engine.computeState(checkpointSum); err != nil {
				t.Fatal(err)
			}
			entry := testBFT.engine.caches.justifier.Remove(checkpointSum.Header.ID())
			if entry == nil {
				t.Fatal("expected checkpoint justifier in cache")
			}
			js := entry.Value.(*justifier)

			assert.Equal(t, tc.expectedPost, js.votes[p.Address].weight,
				"checkpoint block's own vote must use post-housekeep weight, matching thresholdWeight's basis")
			// P is the round's only COM voter so far: comWeight is exactly its weight.
			assert.Equal(t, tc.expectedPost, js.comWeight, "comWeight must reflect the post-housekeep weight")

			totalWeight, err := testBFT.engine.getTotalWeight(checkpointSum)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, totalWeight*2/3, js.thresholdWeight, "threshold must come from the same checkpoint state")

			// Rebuilding the round from its storePoint (as Resync does) must weigh
			// P's checkpoint vote identically.
			storePoint := checkpoint + thor.EpochLength() - 1
			require.NoError(t, testBFT.fastForward(storePoint-testBFT.repo.BestBlockSummary().Header.Number()))
			spSum, err := testBFT.repo.NewBestChain().GetBlockSummary(storePoint)
			if err != nil {
				t.Fatal(err)
			}
			testBFT.engine.caches.state.Purge()
			testBFT.engine.caches.justifier = cache.NewPrioCache(16)
			if _, err := testBFT.engine.computeState(spSum); err != nil {
				t.Fatal(err)
			}
			entry = testBFT.engine.caches.justifier.Remove(spSum.Header.ID())
			if entry == nil {
				t.Fatal("expected storePoint justifier in cache")
			}
			js = entry.Value.(*justifier)
			assert.Equal(t, tc.expectedPost, js.votes[p.Address].weight,
				"rebuilding from the storePoint must use the same post-housekeep weight")
		})
	}
}

// TestComputeStateSignerAbsentFromCommitteeErrors pins that a signer with no
// validation entry in the round's weight state is an invariant violation and errors
// — scheduling and proposer validation both run after SyncPOS, so every signer of a
// PoS round is drawn from its post-housekeep committee; an absent signer means
// corrupted state, not a zero vote.
func TestComputeStateSignerAbsentFromCommitteeErrors(t *testing.T) {
	forkCfg := &thor.ForkConfig{
		HAYABUSA: 1,
	}
	hayabusaTP := uint32(1)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})

	testBFT, err := newTestBftPos(forkCfg)
	if err != nil {
		t.Fatal(err)
	}
	numBlksNeededForPos := forkCfg.HAYABUSA + thor.HayabusaTP() + 1
	if err = testBFT.fastForward(thor.EpochLength() + 1 - numBlksNeededForPos); err != nil {
		t.Fatal(err)
	}

	bestChain := testBFT.repo.NewBestChain()
	checkpointSum, err := bestChain.GetBlockSummary(thor.EpochLength())
	if err != nil {
		t.Fatal(err)
	}
	targetSum, err := bestChain.GetBlockSummary(thor.EpochLength() + 1)
	if err != nil {
		t.Fatal(err)
	}
	genesisSum, err := testBFT.repo.GetBlockSummary(testBFT.repo.GenesisBlock().Header().ID())
	if err != nil {
		t.Fatal(err)
	}

	// Weight state = genesis: no validator registered, every signer is absent.
	js := newJustifier(0, thor.EpochLength(), 0, 1000)
	js.posActive = true
	js.weightRoot = genesisSum.Root()

	testBFT.engine.caches.state.Purge()
	testBFT.engine.caches.justifier = cache.NewPrioCache(16)
	testBFT.engine.caches.justifier.Set(checkpointSum.Header.ID(), js, float64(thor.EpochLength()))

	_, err = testBFT.engine.computeState(targetSum)
	require.Error(t, err, "absent signer must error")
	assert.Contains(t, err.Error(), "absent from committee")
}
