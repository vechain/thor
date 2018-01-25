package schedule

import (
	"encoding/binary"
	"errors"

	"github.com/vechain/thor/schedule/shuffle"
	"github.com/vechain/thor/thor"
)

// Schedule arrange when a proposer to build a block.
type Schedule struct {
	proposers    []Proposer
	parentNumber uint32
	parentTime   uint64
}

// New create a new schedule instance.
func New(
	proposers []Proposer,
	parentNumber uint32,
	parentTime uint64) *Schedule {

	return &Schedule{
		append([]Proposer(nil), proposers...),
		parentNumber,
		parentTime,
	}
}

// Timing to determine time of the proposer to produce a block, according to nowTime.
// If the proposer is not listed, an error returned.
//
// The first return value is the timestamp for the proposer to build a block with.
// It's guaranteed that the timestamp >= nowTime.
//
// The second one is the list of proposers that need to be updated.
func (s *Schedule) Timing(addr thor.Address, nowTime uint64) (
	uint64, //timestamp
	[]Proposer,
	error,
) {
	found := false
	var roundInterval uint64
	for _, p := range s.proposers {
		if p.Address == addr {
			found = true
		}
		if !p.IsAbsent() {
			roundInterval += thor.BlockInterval
		}
	}
	if !found {
		return 0, nil, errors.New("not a proposer")
	}
	predictedTime := s.parentTime + thor.BlockInterval

	var nRound uint64
	if nowTime >= predictedTime+roundInterval {
		nRound = (nowTime - predictedTime) / roundInterval
	}

	updated := make(proposerMap)
	if nRound > 0 {
		// absent all if skip some rounds
		for _, p := range s.proposers {
			if !p.IsAbsent() && p.Address != addr {
				p.SetAbsent(true)
				updated[p.Address] = p
			}
		}
	}

	perm := make([]int, len(s.proposers))
	for {
		// shuffle proposers bases on parent number and round number
		var seed [4 + 8]byte
		binary.BigEndian.PutUint32(seed[:], s.parentNumber)
		binary.BigEndian.PutUint64(seed[4:], nRound)
		shuffle.Shuffle(seed[:], perm)

		timeSlot := predictedTime + roundInterval*nRound

		for _, i := range perm {
			proposer := s.proposers[i]
			if addr == proposer.Address {
				if nowTime > timeSlot {
					// next round
					break
				}
				// update to non-absent if absent
				if proposer.IsAbsent() {
					proposer.SetAbsent(false)
					updated[proposer.Address] = proposer
				}
				return timeSlot, updated.toSlice(), nil
			}

			// step time if proposer not previously absent
			if !proposer.IsAbsent() {
				timeSlot += thor.BlockInterval

				// and add to update list
				proposer.SetAbsent(true)
				updated[proposer.Address] = proposer
			}
		}
		nRound++
	}
}

// Validate returns if the timestamp of addr is valid.
// Error returned if addr is not in proposers list.
func (s *Schedule) Validate(addr thor.Address, timestamp uint64) (bool, []Proposer, error) {
	t, absentee, err := s.Timing(addr, timestamp)
	if err != nil {
		return false, nil, err
	}
	return t == timestamp, absentee, nil
}

// CalcScore calculates score of proposers status.
func CalcScore(all []Proposer, updates []Proposer) uint64 {
	absentee := make(map[thor.Address]interface{})
	for _, p := range all {
		if p.IsAbsent() {
			absentee[p.Address] = nil
		}
	}

	for _, p := range updates {
		if p.IsAbsent() {
			absentee[p.Address] = nil
		} else {
			delete(absentee, p.Address)
		}
	}
	return uint64(len(all) - len(absentee))
}

type proposerMap map[thor.Address]Proposer

func (pm proposerMap) toSlice() []Proposer {
	slice := make([]Proposer, 0, len(pm))
	for _, p := range pm {
		slice = append(slice, p)
	}
	return slice
}
