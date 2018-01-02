package schedule

import (
	"encoding/binary"
	"errors"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/consensus/shuffle"
	"github.com/vechain/thor/params"
)

// Schedule arrange when a proposer to build a block.
type Schedule struct {
	proposers    []acc.Address
	absenteeMap  addrMap
	parentNumber uint32
	parentTime   uint64
}

// New create a new schedule instance.
func New(
	proposers []acc.Address,
	absentee []acc.Address,
	parentNumber uint32,
	parentTime uint64) *Schedule {

	if !(len(absentee) < len(proposers)) {
		panic("len(absentee) must < len(proposers)")
	}

	absenteeMap := map[acc.Address]bool{}
	for _, a := range absentee {
		absenteeMap[a] = true
	}
	return &Schedule{
		append([]acc.Address(nil), proposers...),
		absenteeMap,
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
// The second one is a new absentee list.
func (s *Schedule) Timing(addr acc.Address, nowTime uint64) (
	uint64, //timestamp
	[]acc.Address, //absentee
	error,
) {
	found := false
	for _, a := range s.proposers {
		if a == addr {
			found = true
			break
		}
	}
	if !found {
		return 0, nil, errors.New("not a proposer")
	}

	predictedTime := s.parentTime + params.BlockInterval
	roundInterval := uint64(len(s.proposers)-len(s.absenteeMap)) * params.BlockInterval

	var nRound uint64
	if nowTime >= predictedTime+roundInterval {
		nRound = (nowTime - predictedTime) / roundInterval
	}

	retAbsenteeMap := addrMap{}
	if nRound > 0 {
		// absent all if skip some rounds
		for _, a := range s.proposers {
			retAbsenteeMap[a] = true
		}
	} else {
		// keep absent input absentee
		for a := range s.absenteeMap {
			retAbsenteeMap[a] = true
		}
	}

	perm := make([]int, len(s.proposers))
	for {
		// shuffle proposers bases on parent number and round number
		var seed [4 + 8]byte
		binary.BigEndian.PutUint32(seed[:], s.parentNumber)
		binary.BigEndian.PutUint64(seed[4:], nRound)
		shuffle.Shuffle(seed[:], perm)

		t := predictedTime + roundInterval*nRound

		for _, i := range perm {
			proposer := s.proposers[i]
			if addr != proposer {
				// step time if proposer not in absentee list
				if !s.absenteeMap[proposer] {
					t += params.BlockInterval
				}
				retAbsenteeMap[proposer] = true
				continue
			}

			if nowTime > t {
				// next round
				break
			}
			// kick off proposer in absentee list
			retAbsenteeMap[proposer] = false
			return t, retAbsenteeMap.toSlice(), nil
		}
		nRound++
	}
}

// Validate returns if the timestamp of addr is valid.
// Error returned if addr is not in proposers list.
func (s *Schedule) Validate(addr acc.Address, timestamp uint64) (bool, error) {
	t, _, err := s.Timing(addr, timestamp)
	if err != nil {
		return false, err
	}
	return t == timestamp, nil
}

type addrMap map[acc.Address]bool

func (am addrMap) toSlice() (slice []acc.Address) {
	for a, b := range am {
		if b {
			slice = append(slice, a)
		}
	}
	return
}
