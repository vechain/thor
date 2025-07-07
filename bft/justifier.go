// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"math/big"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
)

// bftState is the summary of a bft round for a given head.
type bftState struct {
	Quality   uint32 // accumulated justified block count
	Justified bool
	Committed bool
}

// justifier tracks all block vote in one bft round and justify the round.
type justifier struct {
	parentQuality uint32
	checkpoint    uint32
	threshold     *big.Int

	votes       map[thor.Address]bool
	voterStakes map[thor.Address]*big.Int
	comVotes    uint64
	comStake    *big.Int
}

func (engine *Engine) newJustifier(parentID thor.Bytes32) (*justifier, error) {
	blockNum := block.Number(parentID) + 1

	var lastOfParentRound uint32
	checkpoint := getCheckPoint(blockNum)
	if checkpoint > 0 {
		lastOfParentRound = checkpoint - 1
	} else {
		lastOfParentRound = 0
	}

	sum, err := engine.repo.NewChain(parentID).GetBlockSummary(lastOfParentRound)
	if err != nil {
		return nil, err
	}
	totalStake, err := engine.getTotalStake(sum)
	if err != nil {
		return nil, err
	}

	threshold := new(big.Int).Mul(totalStake, big.NewInt(2))
	threshold.Div(threshold, big.NewInt(3))

	var parentQuality uint32 // quality of last round
	if absRound := blockNum/thor.CheckpointInterval - engine.forkConfig.FINALITY/thor.CheckpointInterval; absRound == 0 {
		parentQuality = 0
	} else {
		var err error
		parentQuality, err = engine.getQuality(sum.Header.ID())
		if err != nil {
			return nil, err
		}
	}

	return &justifier{
		votes:         make(map[thor.Address]bool),
		voterStakes:   make(map[thor.Address]*big.Int),
		parentQuality: parentQuality,
		checkpoint:    checkpoint,
		threshold:     threshold,
		comStake:      big.NewInt(0),
	}, nil
}

// AddBlock adds a new block to the set.
func (js *justifier) AddBlock(signer thor.Address, isCOM bool, stake *big.Int) {
	if prev, ok := js.votes[signer]; !ok {
		js.votes[signer] = isCOM
		js.voterStakes[signer] = new(big.Int).Set(stake)
		if isCOM {
			js.comVotes++
			js.comStake.Add(js.comStake, stake)
		}
	} else if prev != isCOM {
		// if one votes both COM and non-COM in one round, count as non-COM
		js.votes[signer] = false
		if prev {
			js.comVotes--
			js.comStake.Sub(js.comStake, js.voterStakes[signer])
		}
	}
}

// Summarize summarizes the state of vote set.
func (js *justifier) Summarize() *bftState {
	totalVoterStake := big.NewInt(0)
	for _, stake := range js.voterStakes {
		totalVoterStake.Add(totalVoterStake, stake)
	}

	// Adjust threshold based on actual participating validators
	// This ensures consistency between total stake calculation and vote processing
	adjustedThreshold := js.adjustThresholdForParticipatingValidators()

	justified := totalVoterStake.Cmp(adjustedThreshold) > 0
	committed := js.comStake.Cmp(adjustedThreshold) > 0

	var quality uint32
	if justified {
		quality = js.parentQuality + 1
	} else {
		quality = js.parentQuality
	}

	return &bftState{
		Quality:   quality,
		Justified: justified,
		Committed: committed,
	}
}

// adjustThresholdForParticipatingValidators adjusts the threshold based on
// the proportion of validators that actually participated in voting
func (js *justifier) adjustThresholdForParticipatingValidators() *big.Int {
	// Calculate total stake of participating validators
	totalParticipatingStake := big.NewInt(0)
	for _, stake := range js.voterStakes {
		totalParticipatingStake.Add(totalParticipatingStake, stake)
	}

	// If we have participating validators with stake, adjust threshold proportionally
	// This ensures that the threshold is based on the actual stake that can vote
	if totalParticipatingStake.Sign() > 0 {
		// Calculate 2/3 of the participating stake
		adjustedThreshold := new(big.Int).Mul(totalParticipatingStake, big.NewInt(2))
		adjustedThreshold.Div(adjustedThreshold, big.NewInt(3))
		return adjustedThreshold
	}

	// Fallback to original threshold if no participating stake
	return js.threshold
}
