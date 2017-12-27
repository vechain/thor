package schedule

import (
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/consensus/shuffle"
	"github.com/vechain/thor/params"
)

// Entry presents the time for a block proposer to build a block.
type Entry struct {
	Time     uint64
	Proposer acc.Address
}

// Schedule the entry list of when proposers' turn to build a block.
type Schedule []Entry

// New create a schedule bases on parent block information.
func New(
	proposers []acc.Address,
	absentee []acc.Address,
	parentNumber uint32,
	parentTime uint64) Schedule {

	if len(proposers) == 0 {
		return nil
	}

	// for fast lookup absence
	absenteeMap := make(map[acc.Address]bool, len(absentee))
	for _, a := range absentee {
		absenteeMap[a] = true
	}

	// make a shuffled permutation to shuffle proposers
	perm := make([]int, len(proposers))
	shuffle.Shuffle(parentNumber, perm)

	time := parentTime + params.BlockTime
	entries := make([]Entry, len(proposers))
	for i, j := range perm {
		w := proposers[j]
		entries[i] = Entry{
			time,
			proposers[j],
		}
		// allow one block proposer to occupy timing of previously absent proposer
		if !absenteeMap[w] {
			time += params.BlockTime
		}
	}
	return entries
}

// EntryOf returns the Entry for given proposer.
// If the given proposer is not listed, (nil, -1) returned.
func (s Schedule) EntryOf(proposer acc.Address) (*Entry, int) {
	for i, e := range s {
		if e.Proposer == proposer {
			return &e, i
		}
	}
	return nil, -1
}
