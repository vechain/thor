// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pos

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"math/big"
	"math/rand"
	"slices"
	"sort"

	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/thor"
)

// Scheduler to schedule the time when a proposer to produce a block for PoS.
type Scheduler struct {
	proposer        *staker.Validation
	proposerID      thor.Address
	parentBlockTime uint64
	sequence        []thor.Address
}

type onlineProposer struct {
	id         thor.Address
	validation *staker.Validation
	hash       thor.Bytes32
	score      float64
}

// NewScheduler create a Scheduler object.
// `addr` is the proposer to be scheduled.
// If `addr` is not listed in `proposers` or not active, an error returned.
func NewScheduler(
	addr thor.Address,
	proposers map[thor.Address]*staker.Validation,
	parentBlockNumber uint32,
	parentBlockTime uint64,
	seed []byte,
) (*Scheduler, error) {
	var (
		proposer   *staker.Validation
		proposerID thor.Address
	)
	var num [4]byte
	binary.BigEndian.PutUint32(num[:], parentBlockNumber)

	online := make([]*onlineProposer, 0)
	for id, p := range proposers {
		if id == addr {
			proposer = p
			proposerID = id
		}
		if p.Online || id == addr {
			online = append(online, &onlineProposer{
				id:         id,
				validation: p,
				hash:       thor.Blake2b(seed, num[:], id.Bytes()),
			})
		}
	}

	if proposerID.IsZero() {
		return nil, errors.New("unauthorized block proposer")
	}

	// initial sort -> this is required to ensure the same order for the random source generator
	slices.SortFunc(online, func(i, j *onlineProposer) int {
		return bytes.Compare(i.hash.Bytes(), j.hash.Bytes())
	})

	return &Scheduler{
		proposer,
		proposerID,
		parentBlockTime,
		createSequence(online, seed, parentBlockNumber),
	}, nil
}

// Schedule to determine time of the proposer to produce a block, according to `nowTime`.
// `newBlockTime` is promised to be >= nowTime and > parentBlockTime
func (s *Scheduler) Schedule(nowTime uint64) (newBlockTime uint64) {
	const T = thor.BlockInterval

	newBlockTime = s.parentBlockTime + T
	if nowTime > newBlockTime {
		// ensure T aligned, and >= nowTime
		newBlockTime += (nowTime - newBlockTime + T - 1) / T * T
	}

	offset := (newBlockTime-s.parentBlockTime)/T - 1
	for i, n := uint64(0), uint64(len(s.sequence)); i < n; i++ {
		index := (i + offset) % n
		if s.sequence[index] == s.proposerID {
			return newBlockTime + i*T
		}
	}

	// should never happen
	panic("something wrong with proposers list")
}

// IsTheTime returns if the newBlockTime is correct for the proposer.
func (s *Scheduler) IsTheTime(newBlockTime uint64) bool {
	return s.IsScheduled(newBlockTime, s.proposerID)
}

// IsScheduled returns if the schedule(proposer, blockTime) is correct.
func (s *Scheduler) IsScheduled(blockTime uint64, proposer thor.Address) bool {
	if s.parentBlockTime >= blockTime {
		// invalid block time
		return false
	}

	T := thor.BlockInterval
	if (blockTime-s.parentBlockTime)%T != 0 {
		// invalid block time
		return false
	}

	index := (blockTime - s.parentBlockTime - T) / T % uint64(len(s.sequence))
	return s.sequence[index] == proposer
}

// Updates returns proposers whose status are changed, and the score when new block time is assumed to be newBlockTime.
func (s *Scheduler) Updates(newBlockTime uint64) (map[thor.Address]bool, uint64) {
	T := thor.BlockInterval

	updates := make(map[thor.Address]bool)

	for i := uint64(0); i < uint64(len(s.sequence)); i++ {
		if s.parentBlockTime+T+i*T >= newBlockTime {
			break
		}
		id := s.sequence[i]
		if s.sequence[i] != s.proposerID {
			updates[id] = false
		}
	}

	score := uint64(len(s.sequence) - len(updates))

	if !s.proposer.Online {
		updates[s.proposerID] = true
	}
	return updates, score
}

// createSequence implements a Weighted Random Sampling algorithm using the Exponential Distribution Method.
func createSequence(proposers []*onlineProposer, seed []byte, parentNum uint32) []thor.Address {
	if len(proposers) == 0 {
		return []thor.Address{}
	}

	// Step 1: Generate a deterministic seed for the random number generator
	var num [4]byte
	binary.BigEndian.PutUint32(num[:], parentNum)

	hashedSeed := thor.Blake2b(seed, num[:])
	randomSource := rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(hashedSeed[:])))) //nolint:gosec

	// Step 2: Calculate priority scores for each validator based on their weight
	// using the exponential distribution method for weighted random sampling
	weightedProposers := make([]*onlineProposer, len(proposers))
	bigE18 := big.NewFloat(1e18) // Divisor constant to convert from wei to ether (10^18)

	for i, proposer := range proposers {
		// Convert weight from wei to a manageable float value
		weight := new(big.Float).SetInt(proposer.validation.Weight)
		weight = weight.Quo(weight, bigE18)
		weightFloat, _ := weight.Float64()
		if weightFloat < 1 {
			weightFloat = 1 // Ensure a minimum weight threshold
		}

		// Generate random value and calculate priority using exponential distribution
		randomValue := randomSource.Float64()
		priorityScore := math.Pow(randomValue, 1.0/weightFloat)

		weightedProposers[i] = &onlineProposer{
			id:         proposer.id,
			score:      priorityScore,
			hash:       proposer.hash,
			validation: proposer.validation,
		}
	}

	// Step 3: Sort validators by priority score in descending order
	sort.Slice(weightedProposers, func(i, j int) bool {
		return weightedProposers[i].score > weightedProposers[j].score
	})

	// Step 4: Extract the validator IDs in priority order
	resultSequence := make([]thor.Address, len(weightedProposers))
	for i, validator := range weightedProposers {
		resultSequence[i] = validator.id
	}

	return resultSequence
}
