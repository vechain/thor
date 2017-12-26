package schedule

import (
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/consensus/shuffle"
	"github.com/vechain/thor/params"
)

// Entry presents the time for a witness to build a block.
type Entry struct {
	Time    uint64
	Witness acc.Address
}

// Schedule the entry list of when witnesses' turn to build a block.
type Schedule []Entry

// New create a schedule bases on parent block information.
func New(
	witnesses []acc.Address,
	absentee []acc.Address,
	parentNumber uint32,
	parentTime uint64) Schedule {

	if len(witnesses) == 0 {
		return nil
	}

	// for fast lookup absence
	absenteeMap := make(map[acc.Address]bool, len(absentee))
	for _, a := range absentee {
		absenteeMap[a] = true
	}

	// make a shuffled permutation to shuffle witnesses
	perm := make([]int, len(witnesses))
	shuffle.Shuffle(parentNumber, perm)

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
	return entries
}

// EntryOf returns the Entry for given witness.
// If the given witness is not listed, (nil, -1) returned.
func (s Schedule) EntryOf(witness acc.Address) (*Entry, int) {
	for i, e := range s {
		if e.Witness == witness {
			return &e, i
		}
	}
	return nil, -1
}
