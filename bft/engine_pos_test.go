// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"testing"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

var (
	validatorStake   = new(big.Int).Mul(big.NewInt(25_000_000), big.NewInt(1e18))
	minStakingPeriod = uint32(360) * 24 * 7
)

func init() {
	defaultFC.FINALITY = 0
}

func TestFinalizedPos(t *testing.T) {
	forkCfg := &thor.ForkConfig{
		HAYABUSA:    1,
		HAYABUSA_TP: 1,
	}

	testBFT, err := newTestBftPos(forkCfg)
	if err != nil {
		t.Fatal(err)
	}

	numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)
	if err = testBFT.fastForward(thor.CheckpointInterval*3 - 1 - numBlksNeededForPos); err != nil {
		t.Fatal(err)
	}

	sum, err := testBFT.repo.NewBestChain().GetBlockSummary(uint32(MaxBlockProposers*2/3 + 1))
	if err != nil {
		t.Fatal(err)
	}

	st, err := testBFT.engine.computeState(sum.Header)
	if err != nil {
		t.Fatal(err)
	}

	// should be justify and commit at firstBlockNum
	assert.Equal(t, uint32(1), st.Quality)
	assert.True(t, st.Justified)
	assert.True(t, st.Committed)

	blockNum := uint32(thor.CheckpointInterval*2 + MaxBlockProposers*2/3)

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

func TestAcceptsPos(t *testing.T) {
	forkCfg := &thor.ForkConfig{
		HAYABUSA:    1,
		HAYABUSA_TP: 1,
	}

	testBFT, err := newTestBftPos(forkCfg)
	if err != nil {
		t.Fatal(err)
	}

	numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)
	if err = testBFT.fastForward(thor.CheckpointInterval - 1 - numBlksNeededForPos); err != nil {
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

				numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)
				testBFT.fastForwardWithMinority(thor.CheckpointInterval*3 - numBlksNeededForPos)
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

				numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)
				testBFT.fastForward(thor.CheckpointInterval*3 - numBlksNeededForPos)
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

				numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)
				if err = testBFT.fastForward(thor.CheckpointInterval*3 - 1 - numBlksNeededForPos); err != nil {
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

				numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)
				if err = testBFT.fastForward(thor.CheckpointInterval*3 - 1 - numBlksNeededForPos); err != nil {
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

				numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)
				if err = testBFT.fastForward(thor.CheckpointInterval*3 - 1 - numBlksNeededForPos); err != nil {
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

				numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)
				testBFT.fastForwardWithMinority(thor.CheckpointInterval*3 - numBlksNeededForPos)
				testBFT.fastForward(thor.CheckpointInterval*1 + 3)
				_, err = testBFT.engine.ShouldVote(testBFT.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	forkCfg := &thor.ForkConfig{
		HAYABUSA:    1,
		HAYABUSA_TP: 1,
	}

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

				numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)
				testBft.fastForward(thor.CheckpointInterval - 1 - numBlksNeededForPos)

				vs, err := testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				if forkCfg.HAYABUSA != thor.NoFork.HAYABUSA {
					assert.Equal(t, uint32(180), vs.checkpoint)
					expected, ok := new(big.Int).SetString("166666666666666666666666666", 10)
					assert.True(t, ok)
					assert.Equal(t, expected, vs.thresholdWeight)
				} else {
					assert.Equal(t, uint32(0), vs.checkpoint)
					assert.Equal(t, uint64(MaxBlockProposers*2/3), vs.thresholdVotes)
				}
			},
		}, {
			"fork in the middle of checkpoint", func(t *testing.T, forkCfg *thor.ForkConfig) {
				fc := *forkCfg
				fc.VIP214 = thor.CheckpointInterval / 2
				testBft, err := newTestBftPos(&fc)
				if err != nil {
					t.Fatal(err)
				}
				vs, err := testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, uint32(0), vs.checkpoint)
				assert.Equal(t, uint64(MaxBlockProposers*2/3), vs.thresholdVotes)
			},
		}, {
			"the second bft round", func(t *testing.T, forkCfg *thor.ForkConfig) {
				fc := *forkCfg
				fc.VIP214 = thor.CheckpointInterval / 2
				testBft, err := newTestBftPos(&fc)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)
				testBft.fastForward(thor.CheckpointInterval*2 - numBlksNeededForPos)
				vs, err := testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, uint32(thor.CheckpointInterval*2), vs.checkpoint)
				expected, ok := new(big.Int).SetString("166666666666666666666666666", 10)
				assert.True(t, ok)
				assert.Equal(t, expected, vs.thresholdWeight)
				assert.Equal(t, uint32(2), vs.Summarize().Quality)
				assert.False(t, vs.Summarize().Justified)
				assert.False(t, vs.Summarize().Committed)
			},
		}, {
			"add votes: commits", func(t *testing.T, forkCfg *thor.ForkConfig) {
				fc := *forkCfg
				fc.VIP214 = thor.CheckpointInterval / 2
				testBft, err := newTestBftPos(&fc)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)
				testBft.fastForward(thor.CheckpointInterval*2 - 1 - numBlksNeededForPos)
				vs, err := testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				for i := 0; i <= MaxBlockProposers*2/3; i++ {
					// Weight, stake multiplied by default multiplier
					vs.AddBlock(datagen.RandAddress(), true, new(big.Int).Mul(validatorStake, big.NewInt(2)))
				}

				st := vs.Summarize()
				assert.Equal(t, uint32(3), st.Quality)
				assert.True(t, st.Justified)
				assert.True(t, st.Committed)

				// add vote after commits，commit/justify stays the same
				vs.AddBlock(datagen.RandAddress(), true, new(big.Int).Mul(validatorStake, big.NewInt(2)))

				st = vs.Summarize()
				assert.Equal(t, uint32(3), st.Quality)
				assert.True(t, st.Justified)
				assert.True(t, st.Committed)
			},
		}, {
			"add votes: justifies", func(t *testing.T, forkCfg *thor.ForkConfig) {
				fc := *forkCfg
				fc.VIP214 = thor.CheckpointInterval / 2
				testBft, err := newTestBftPos(&fc)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)
				testBft.fastForward(thor.CheckpointInterval*2 - 1 - numBlksNeededForPos)
				vs, err := testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				for i := 0; i <= MaxBlockProposers*2/3; i++ {
					// Weight, stake multiplied by default multiplier
					vs.AddBlock(datagen.RandAddress(), false, new(big.Int).Mul(validatorStake, big.NewInt(2)))
				}

				st := vs.Summarize()
				assert.Equal(t, uint32(3), st.Quality)
				assert.True(t, st.Justified)
				assert.False(t, st.Committed)
			},
		}, {
			"add votes: one votes WIT then changes to COM", func(t *testing.T, forkCfg *thor.ForkConfig) {
				fc := *forkCfg
				fc.VIP214 = thor.CheckpointInterval / 2
				testBft, err := newTestBftPos(&fc)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)
				testBft.fastForward(thor.CheckpointInterval*2 - 1 - numBlksNeededForPos)
				vs, err := testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
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
					vs.AddBlock(master, false, nil)
				}

				// justifies but not committed
				st := vs.Summarize()
				assert.True(t, st.Justified)
				assert.False(t, st.Committed)

				// master votes COM
				if forkCfg.HAYABUSA != thor.NoFork.HAYABUSA {
					vs.AddBlock(master, true, new(big.Int).Mul(validatorStake, big.NewInt(2)))
				} else {
					vs.AddBlock(master, true, nil)
				}

				// should not be committed
				st = vs.Summarize()
				assert.False(t, st.Committed)

				// another master votes WIT
				if forkCfg.HAYABUSA != thor.NoFork.HAYABUSA {
					vs.AddBlock(datagen.RandAddress(), true, new(big.Int).Mul(validatorStake, big.NewInt(2)))
				} else {
					vs.AddBlock(datagen.RandAddress(), true, nil)
				}
				st = vs.Summarize()
				assert.True(t, st.Committed)
			},
		}, {
			"vote both WIT and COM in one round", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBft, err := newTestBftPos(forkCfg)
				if err != nil {
					t.Fatal(err)
				}
				vs, err := testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}

				master := datagen.RandAddress()
				vs.AddBlock(master, true, new(big.Int).Mul(validatorStake, big.NewInt(2)))
				assert.Equal(t, true, vs.votes[master].isCOM)
				assert.Equal(t, uint64(1), vs.comVotes)

				vs.AddBlock(master, false, new(big.Int).Mul(validatorStake, big.NewInt(2)))
				assert.Equal(t, false, vs.votes[master].isCOM)
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(master, true, new(big.Int).Mul(validatorStake, big.NewInt(2)))

				assert.Equal(t, false, vs.votes[master].isCOM)
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(master, false, new(big.Int).Mul(validatorStake, big.NewInt(2)))

				assert.Equal(t, false, vs.votes[master].isCOM)
				assert.Equal(t, uint64(0), vs.comVotes)

				vs, err = testBft.engine.newJustifier(testBft.repo.BestBlockSummary().Header.ID())
				if err != nil {
					t.Fatal(err)
				}
				vs.AddBlock(master, false, new(big.Int).Mul(validatorStake, big.NewInt(2)))

				assert.Equal(t, false, vs.votes[master].isCOM)
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(master, true, new(big.Int).Mul(validatorStake, big.NewInt(2)))

				assert.Equal(t, false, vs.votes[master].isCOM)
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(master, true, new(big.Int).Mul(validatorStake, big.NewInt(2)))

				assert.Equal(t, false, vs.votes[master].isCOM)
				assert.Equal(t, uint64(0), vs.comVotes)

				vs.AddBlock(master, false, new(big.Int).Mul(validatorStake, big.NewInt(2)))

				assert.Equal(t, false, vs.votes[master].isCOM)
				assert.Equal(t, uint64(0), vs.comVotes)
			},
		},
	}

	forkCfg := &thor.ForkConfig{
		HAYABUSA:    1,
		HAYABUSA_TP: 1,
	}

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

				for range 3 * thor.CheckpointInterval {
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

				numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)
				if err = testBFT.fastForward(thor.CheckpointInterval - numBlksNeededForPos); err != nil {
					t.Fatal(err)
				}

				for range thor.CheckpointInterval - 2 {
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
			"first three not justified rounds, then justified", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBFT, err := newTestBftPos(forkCfg)
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
			"get finalized, then justified", func(t *testing.T, forkCfg *thor.ForkConfig) {
				testBFT, err := newTestBftPos(forkCfg)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)

				if err = testBFT.fastForward(3*thor.CheckpointInterval - 1 - numBlksNeededForPos); err != nil {
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
			"get finalized, not justified, then justified", func(t *testing.T, forkCfg *thor.ForkConfig) {
				type tJustified = justified
				testBFT, err := newTestBftPos(forkCfg)
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
			"fork in the middle, get justified", func(t *testing.T, forkCfg *thor.ForkConfig) {
				fc := *forkCfg
				fc.FINALITY = thor.CheckpointInterval

				testBFT, err := newTestBftPos(&fc)
				if err != nil {
					t.Fatal(err)
				}

				numBlksNeededForPos := int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP + 1)
				if err = testBFT.fastForward(thor.CheckpointInterval - numBlksNeededForPos); err != nil {
					t.Fatal(err)
				}

				for range thor.CheckpointInterval - 2 {
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

	forkCfg := &thor.ForkConfig{
		HAYABUSA:    1,
		HAYABUSA_TP: 1,
	}

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
	staker.EpochLength = solidity.NewConfigVariable("epoch-length", 1)

	if err = testBFT.fastForward(int(forkCfg.HAYABUSA + forkCfg.HAYABUSA_TP)); err != nil {
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
	flow, _, err := packer.Mock(parentSummary, parentSummary.Header.Timestamp()+thor.BlockInterval, parentSummary.Header.GasLimit())
	if err != nil {
		return nil, err
	}

	conflicts, err := test.repo.ScanConflicts(parentSummary.Header.Number() + 1)
	if err != nil {
		return nil, err
	}

	staker := builtin.Staker.Native(test.stater.NewState(parentSummary.Root()))
	validation, err := staker.Get(master.Address)
	if err != nil {
		return nil, err
	}
	if validation.IsEmpty() {
		// Add all dev accounts as validators
		for _, dev := range devAccounts {
			if err := test.adoptStakerTx(flow, dev.PrivateKey, "addValidation", validatorStake, dev.Address, minStakingPeriod); err != nil {
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

	if b.Header().Number() >= test.fc.FINALITY {
		if err = test.engine.CommitBlock(b.Header(), false); err != nil {
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
