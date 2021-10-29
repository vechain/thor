// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package poa

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sort"

	"github.com/vechain/thor/thor"
)

// SchedulerV2 to schedule the time when a proposer to produce a block.
// V2 is for post VIP-214 stage.
type SchedulerV2 struct {
	proposer        Proposer
	parentBlockTime uint64
	shuffled        []thor.Address
}

var _ Scheduler = (*SchedulerV2)(nil)

// NewSchedulerV2 create a SchedulerV2 object.
// `addr` is the proposer to be scheduled.
// If `addr` is not listed in `proposers` or not active, an error returned.
func NewSchedulerV2(
	addr thor.Address,
	proposers []Proposer,
	parentBlockNumber uint32,
	parentBlockTime uint64,
	seed []byte) (*SchedulerV2, error) {

	var (
		listed   = false
		proposer Proposer
		list     []struct {
			addr thor.Address
			hash thor.Bytes32
		}
	)
	var num [4]byte
	binary.BigEndian.PutUint32(num[:], parentBlockNumber)

	for _, p := range proposers {
		if p.Address == addr {
			proposer = p
			listed = true
		}
		if p.Active || p.Address == addr {
			list = append(list, struct {
				addr thor.Address
				hash thor.Bytes32
			}{
				p.Address,
				thor.Blake2b(seed, num[:], p.Address.Bytes()),
			})
		}
	}

	if !listed {
		return nil, errors.New("unauthorized block proposer")
	}

	sort.Slice(list, func(i, j int) bool {
		return bytes.Compare(list[i].hash.Bytes(), list[j].hash.Bytes()) < 0
	})

	shuffled := make([]thor.Address, 0, len(list))
	for _, t := range list {
		shuffled = append(shuffled, t.addr)
	}

	return &SchedulerV2{
		proposer,
		parentBlockTime,
		shuffled,
	}, nil
}

// Schedule to determine time of the proposer to produce a block, according to `nowTime`.
// `newBlockTime` is promised to be >= nowTime and > parentBlockTime
func (s *SchedulerV2) Schedule(nowTime uint64) (newBlockTime uint64) {
	const T = thor.BlockInterval

	newBlockTime = s.parentBlockTime + T
	if nowTime > newBlockTime {
		// ensure T aligned, and >= nowTime
		newBlockTime += (nowTime - newBlockTime + T - 1) / T * T
	}

	offset := (newBlockTime-s.parentBlockTime)/T - 1
	for i, n := uint64(0), uint64(len(s.shuffled)); i < n; i++ {
		index := (i + offset) % n
		if s.shuffled[index] == s.proposer.Address {
			return newBlockTime + i*T
		}
	}

	// should never happen
	panic("something wrong with proposers list")
}

// IsTheTime returns if the newBlockTime is correct for the proposer.
func (s *SchedulerV2) IsTheTime(newBlockTime uint64) bool {
	return s.IsScheduled(newBlockTime, s.proposer.Address)
}

// IsScheduled returns if the schedule(proposer, blockTime) is correct.
func (s *SchedulerV2) IsScheduled(blockTime uint64, proposer thor.Address) bool {
	if s.parentBlockTime >= blockTime {
		// invalid block time
		return false
	}

	T := thor.BlockInterval
	if (blockTime-s.parentBlockTime)%T != 0 {
		// invalid block time
		return false
	}

	index := (blockTime - s.parentBlockTime - T) / T % uint64(len(s.shuffled))
	return s.shuffled[index] == proposer
}

// Updates returns proposers whose status are changed, and the score when new block time is assumed to be newBlockTime.
func (s *SchedulerV2) Updates(newBlockTime uint64) (updates []Proposer, score uint64) {
	T := thor.BlockInterval

	for i := uint64(0); i < uint64(len(s.shuffled)); i++ {
		if s.parentBlockTime+T+i*T >= newBlockTime {
			break
		}
		if s.shuffled[i] != s.proposer.Address {
			updates = append(updates, Proposer{Address: s.shuffled[i], Active: false})
		}
	}

	score = uint64(len(s.shuffled) - len(updates))

	if !s.proposer.Active {
		cpy := s.proposer
		cpy.Active = true
		updates = append(updates, cpy)
	}
	return
}
