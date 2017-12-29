package schedule

import (
	"errors"

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
type Schedule struct {
	entries []Entry
}

// New create a schedule bases on parent block information.
func New(
	proposers []acc.Address,
	absentee []acc.Address,
	parentNumber uint32,
	parentTime uint64) *Schedule {

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
	return &Schedule{entries}
}

// Timing to determine time of the proposer to produce a block, according to nowTime.
//
// The first return value is the timestamp to be waited until.
// It's guaranteed that timestamp > nowTime - params.BlockTime.
//
// The second one is a list of proposers that will be absented if
// this proposer builds a block at the returned timestamp.
//
// If the proposer is not listed, an error returned.
func (s *Schedule) Timing(proposer acc.Address, nowTime uint64) (
	uint64, //timestamp
	[]acc.Address, //absentee
	error) {

	var absentee []acc.Address
	for i, e := range s.entries {
		if e.Proposer == proposer {
			if nowTime < e.Time+params.BlockTime {
				return e.Time, absentee, nil
			}
			// out of the range of schedule
			// absent all except self
			for _, ae := range s.entries[i+1:] {
				absentee = append(absentee, ae.Proposer)
			}

			lastEntry := s.entries[len(s.entries)-1]
			if nowTime < lastEntry.Time+params.BlockTime {
				return lastEntry.Time + params.BlockTime, absentee, nil
			}

			rounded := (nowTime - lastEntry.Time) / params.BlockTime * params.BlockTime
			return lastEntry.Time + rounded, absentee, nil
		}
		absentee = append(absentee, e.Proposer)
	}
	return 0, nil, errors.New("not a proposer")
}
