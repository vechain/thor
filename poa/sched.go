package poa

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/vechain/thor/thor"
)

// Scheduler to schedule the time when a proposer to produce a block.
type Scheduler struct {
	proposer          Proposer
	actives           []Proposer
	parentBlockNumber uint32
	parentBlockTime   uint64
}

// NewScheduler create a Scheduler object.
// `addr` is the proposer to be scheduled.
// If `addr` is not listed in `proposers`, an error returned.
func NewScheduler(
	addr thor.Address,
	proposers []Proposer,
	parentBlockNumber uint32,
	parentBlockTime uint64) (*Scheduler, error) {

	actives := make([]Proposer, 0, len(proposers))
	listed := false
	var proposer Proposer
	for _, p := range proposers {
		if p.Address == addr {
			proposer = p
			actives = append(actives, p)
			listed = true
		} else if p.Active {
			actives = append(actives, p)
		}
	}

	if !listed {
		return nil, errors.New("unauthorized block proposer")
	}

	return &Scheduler{
		proposer,
		actives,
		parentBlockNumber,
		parentBlockTime,
	}, nil
}

func (s *Scheduler) whoseTurn(t uint64) Proposer {
	index := dprp(s.parentBlockNumber, t) % uint64(len(s.actives))
	return s.actives[index]
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

	for {
		p := s.whoseTurn(newBlockTime)
		if p.Address == s.proposer.Address {
			return newBlockTime
		}

		// try next time slot
		newBlockTime += T
	}
}

// IsTheTime returns if the newBlockTime is correct for the proposer.
func (s *Scheduler) IsTheTime(newBlockTime uint64) bool {
	if s.parentBlockTime >= newBlockTime {
		// invalid block time
		return false
	}

	if (newBlockTime-s.parentBlockTime)%thor.BlockInterval != 0 {
		// invalid block time
		return false
	}

	return s.whoseTurn(newBlockTime).Address == s.proposer.Address
}

// Updates returns proposers whose status are change, and the score when new block time is assumed to be newBlockTime.
func (s *Scheduler) Updates(newBlockTime uint64) (updates []Proposer, score uint64) {

	toDeactive := make(map[thor.Address]Proposer)

	t := newBlockTime - thor.BlockInterval
	for i := uint64(0); i < thor.MaxBlockProposers && t > s.parentBlockTime; i++ {
		p := s.whoseTurn(t)
		if p.Address != s.proposer.Address {
			toDeactive[p.Address] = p
		}
		t -= thor.BlockInterval
	}

	updates = make([]Proposer, 0, len(toDeactive)+1)
	for _, p := range toDeactive {
		p.Active = false
		updates = append(updates, p)
	}

	if !s.proposer.Active {
		cpy := s.proposer
		cpy.Active = true
		updates = append(updates, cpy)
	}

	score = uint64(len(s.actives)) - uint64(len(toDeactive))
	return
}

// dprp deterministic pseudo-random process.
// H(B, t)[:8]
func dprp(blockNumber uint32, time uint64) uint64 {
	var bin [12]byte
	binary.BigEndian.PutUint32(bin[:], blockNumber)
	binary.BigEndian.PutUint64(bin[4:], time)
	sum := sha256.Sum256(bin[:])
	return binary.BigEndian.Uint64(sum[:])
}
