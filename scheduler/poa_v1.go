// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package scheduler

import (
	"encoding/binary"
	"errors"

	"github.com/vechain/thor/v2/thor"
)

// PoASchedulerV1 to schedule the time when a proposer to produce a block.
type PoASchedulerV1 struct {
	proposer          Proposer
	actives           []Proposer
	parentBlockNumber uint32
	parentBlockTime   uint64
}

var _ Scheduler = (*PoASchedulerV1)(nil)

// NewPoASchedulerV1 create a PoASchedulerV1 object.
// `addr` is the proposer to be scheduled.
// If `addr` is not listed in `proposers`, an error returned.
func NewPoASchedulerV1(
	addr thor.Address,
	proposers []Proposer,
	parentBlockNumber uint32,
	parentBlockTime uint64,
) (*PoASchedulerV1, error) {
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

	return &PoASchedulerV1{
		proposer,
		actives,
		parentBlockNumber,
		parentBlockTime,
	}, nil
}

func (s *PoASchedulerV1) whoseTurn(t uint64) Proposer {
	index := dprp(s.parentBlockNumber, t) % uint64(len(s.actives))
	return s.actives[index]
}

// Schedule to determine time of the proposer to produce a block, according to `nowTime`.
// `newBlockTime` is promised to be >= nowTime and > parentBlockTime
func (s *PoASchedulerV1) Schedule(nowTime uint64) (newBlockTime uint64) {
	T := thor.BlockInterval()

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
func (s *PoASchedulerV1) IsTheTime(newBlockTime uint64) bool {
	if s.parentBlockTime >= newBlockTime {
		// invalid block time
		return false
	}

	if (newBlockTime-s.parentBlockTime)%thor.BlockInterval() != 0 {
		// invalid block time
		return false
	}

	return s.whoseTurn(newBlockTime).Address == s.proposer.Address
}

// Updates returns proposers whose status are changed, and the score when new block time is assumed to be newBlockTime.
func (s *PoASchedulerV1) Updates(newBlockTime uint64) (updates []Proposer, score uint64) {
	T := thor.BlockInterval()
	toDeactivate := make(map[thor.Address]Proposer)

	t := newBlockTime - T
	for i := uint64(0); i < thor.InitialMaxBlockProposers && t > s.parentBlockTime; i++ {
		p := s.whoseTurn(t)
		if p.Address != s.proposer.Address {
			toDeactivate[p.Address] = p
		}
		t -= T
	}

	updates = make([]Proposer, 0, len(toDeactivate)+1)
	for _, p := range toDeactivate {
		p.Active = false
		updates = append(updates, p)
	}

	if !s.proposer.Active {
		cpy := s.proposer
		cpy.Active = true
		updates = append(updates, cpy)
	}

	score = uint64(len(s.actives)) - uint64(len(toDeactivate))
	return
}

// dprp deterministic pseudo-random process.
// H(B, t)[:8]
func dprp(blockNumber uint32, time uint64) uint64 {
	var (
		b4 [4]byte
		b8 [8]byte
	)
	binary.BigEndian.PutUint32(b4[:], blockNumber)
	binary.BigEndian.PutUint64(b8[:], time)

	return binary.BigEndian.Uint64(thor.Blake2b(b4[:], b8[:]).Bytes())
}
