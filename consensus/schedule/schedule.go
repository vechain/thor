package schedule

import (
	"errors"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/consensus/shuffle"
	"github.com/vechain/thor/params"
)

// Entry presents when and who.
type Entry struct {
	Time    uint64
	Witness acc.Address
}

// Schedule the plan of when witnesses' turn to build a block.
type Schedule struct {
	entries []Entry
}

// New create a schedule bases on parent block information.
func New(
	witnesses []acc.Address,
	absentee []acc.Address,
	parentWitness acc.Address,
	parentBlockNumber uint32,
	parentTime uint64) *Schedule {

	if len(witnesses) == 0 {
		return &Schedule{}
	}

	// for fast lookup absence
	absenteeMap := make(map[acc.Address]bool, len(absentee))
	for _, a := range absentee {
		absenteeMap[a] = true
	}

	// make a shuffled permutation to shuffle witnesses
	perm := make([]int, len(witnesses))
	shuffle.Shuffle(parentBlockNumber, perm)

	time := parentTime + params.BlockTime
	entries := make([]Entry, len(witnesses))
	for i, j := range perm {
		w := witnesses[j]
		entries[i] = Entry{
			time,
			witnesses[j],
		}
		// allow one witness to occupy timing of previously absent witness
		if !absenteeMap[w] {
			time += params.BlockTime
		}
	}
	return &Schedule{
		entries,
	}
}

// Timing returns the time when the witness' turn to build a block.
// If the given witness is not listed, an error returned.
func (s *Schedule) Timing(witness acc.Address) (uint64, error) {
	for _, e := range s.entries {
		if e.Witness == witness {
			return e.Time, nil
		}
	}
	return 0, errors.New("no entry")
}
