// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pos

import (
	"encoding/binary"
	"errors"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/vechain/thor/v2/thor"
)

type Proposer struct {
	Address thor.Address
	Active  bool
	Weight  uint64
}

type entry struct {
	address thor.Address
	weight  uint64
	active  bool
	score   float64
}

// Scheduler to schedule the time when a proposer to produce a block for PoS.
type Scheduler struct {
	proposer        Proposer
	parentBlockTime uint64
	sequence        []entry
}

// NewScheduler create a Scheduler object.
// `addr` is the proposer to be scheduled.
// If `addr` is not listed in `proposers`, an error is returned.
func NewScheduler(
	addr thor.Address,
	proposers []Proposer,
	parentBlockNumber uint32,
	parentBlockTime uint64,
	seed []byte,
) (*Scheduler, error) {
	var (
		listed   = false
		proposer Proposer
		shuffled = make([]entry, 0, len(proposers))
	)

	var num [4]byte
	binary.BigEndian.PutUint32(num[:], parentBlockNumber)

	// Step 1: Generate a seed for the deterministic pseudo-random generator
	hashedSeed := thor.Blake2b(seed, num[:])
	src := rand.NewChaCha8(hashedSeed)
	pseudoRND := rand.New(src) //#nosec G404

	// Step 2: Calculate priority scores for each validator based on their weight
	// using the exponential distribution method for weighted random sampling
	for _, p := range proposers {
		if p.Address == addr {
			proposer = p
			listed = true
		}
		// IMPORTANT: Every validator in the leader group should be allocated
		// with the deterministic random number sequence from the same source.
		random := pseudoRND.Float64()
		if random == 0 {
			random = 1e-10 // prevent ln(0)
		}
		// but only active/online validators will be picked for block production
		if p.Active || p.Address == addr {
			shuffled = append(shuffled, entry{
				address: p.Address,
				weight:  p.Weight,
				active:  p.Active,
				score:   -math.Log(random) / float64(p.Weight),
			})
			// https://en.wikipedia.org/wiki/Reservoir_sampling#Algorithm_A-Res
			// A-Res Reservoir sampling algorithm with -ln(random)/weight
		}
	}
	if !listed {
		return nil, errors.New("unauthorized block proposer")
	}

	// Step 3: Sort validators by priority score in ascending order
	slices.SortStableFunc(shuffled, func(a, b entry) int {
		switch {
		case a.score < b.score:
			return -1
		case a.score > b.score:
			return 1
		default:
			return 0
		}
	})

	return &Scheduler{
		proposer,
		parentBlockTime,
		shuffled,
	}, nil
}

// Schedule to determine time of the proposer to produce a block, according to `nowTime`.
// `newBlockTime` is promised to be >= nowTime and > parentBlockTime
func (s *Scheduler) Schedule(nowTime uint64) (newBlockTime uint64) {
	T := thor.BlockInterval()

	newBlockTime = s.parentBlockTime + T
	if nowTime > newBlockTime {
		// ensure T aligned, and >= nowTime
		newBlockTime += (nowTime - newBlockTime + T - 1) / T * T
	}

	offset := (newBlockTime-s.parentBlockTime)/T - 1
	for i, n := uint64(0), uint64(len(s.sequence)); i < n; i++ {
		index := (i + offset) % n
		if s.sequence[index].address == s.proposer.Address {
			return newBlockTime + i*T
		}
	}

	// should never happen
	panic("something wrong with proposers list")
}

// IsTheTime returns if the newBlockTime is correct for the proposer.
func (s *Scheduler) IsTheTime(newBlockTime uint64) bool {
	return s.IsScheduled(newBlockTime, s.proposer.Address)
}

// IsScheduled returns if the schedule(proposer, blockTime) is correct.
func (s *Scheduler) IsScheduled(blockTime uint64, proposer thor.Address) bool {
	if s.parentBlockTime >= blockTime {
		// invalid block time
		return false
	}

	T := thor.BlockInterval()
	if (blockTime-s.parentBlockTime)%T != 0 {
		// invalid block time
		return false
	}

	index := (blockTime - s.parentBlockTime - T) / T % uint64(len(s.sequence))
	return s.sequence[index].address == proposer
}

// Updates returns proposers whose status are changed, and the score when new block time is assumed to be newBlockTime.
func (s *Scheduler) Updates(newBlockTime uint64, totalWeight uint64) ([]Proposer, uint64) {
	T := thor.BlockInterval()

	updates := make([]Proposer, 0)

	// calculate all online weight
	onlineWeight := uint64(0)
	for idx := range s.sequence {
		onlineWeight += s.sequence[idx].weight
	}

	activeWeight := onlineWeight
	for i := uint64(0); i < uint64(len(s.sequence)); i++ {
		if s.parentBlockTime+T+i*T >= newBlockTime {
			break
		}
		if s.sequence[i].address != s.proposer.Address {
			updates = append(updates, Proposer{Address: s.sequence[i].address, Active: false})

			// subtract the weight from active weight
			activeWeight -= s.sequence[i].weight
		}
	}

	if !s.proposer.Active {
		updates = append(updates, Proposer{Address: s.proposer.Address, Active: true})
	}

	if totalWeight > 0 {
		score := activeWeight * thor.MaxPosScore / totalWeight
		return updates, score
	}

	return updates, 0
}

func uint64ToI64(u uint64) int64 {
	return int64(u ^ (1 << 63))
}
