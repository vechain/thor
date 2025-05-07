// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pos

import (
	"bytes"
	"encoding/binary"
	"math/big"
	"sort"

	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/thor"
)

type Scheduler struct {
	validator       *staker.Validation
	id              thor.Bytes32
	parentBlockTime uint64
	validators      map[thor.Bytes32]*staker.Validation
	placements      []placement
	seed            []byte
}

type placement struct {
	start *big.Rat
	end   *big.Rat
	addr  thor.Address
	hash  thor.Bytes32
	id    thor.Bytes32
}

func NewScheduler(
	signer thor.Address,
	validators map[thor.Bytes32]*staker.Validation,
	parentBlockNumber uint32,
	parentBlockTime uint64,
	seed []byte,
) (*Scheduler, error) {
	if len(validators) == 0 {
		return nil, errors.New("no validators")
	}
	var (
		listed      = false
		validator   *staker.Validation
		validatorID thor.Bytes32
	)
	var num [4]byte
	binary.BigEndian.PutUint32(num[:], parentBlockNumber)

	placements := make([]placement, 0, len(validators))
	onlineStake := big.NewInt(0)

	for id, entry := range validators {
		if entry.Master == signer {
			validatorID = id
			validator = entry
			listed = true
		}
		if entry.Online || entry.Master == signer {
			onlineStake.Add(onlineStake, entry.Weight)
			placements = append(placements, placement{
				addr: entry.Master,
				hash: thor.Blake2b(seed, num[:], entry.Master.Bytes()),
				id:   id,
			})
		}
	}

	if !listed {
		return nil, errors.New("unauthorized block proposer")
	}

	sort.Slice(placements, func(i, j int) bool {
		return bytes.Compare(placements[i].hash.Bytes(), placements[j].hash.Bytes()) < 0
	})

	prev := big.NewRat(0, 1)
	totalStakeRat := new(big.Rat).SetInt(onlineStake)

	for i := range placements {
		weightRat := new(big.Rat).SetInt(validators[placements[i].id].Weight)
		weight := new(big.Rat).Quo(weightRat, totalStakeRat)

		placements[i].start = new(big.Rat).Set(prev)
		placements[i].end = new(big.Rat).Add(prev, weight)
		prev = placements[i].end
	}

	return &Scheduler{
		validator,
		validatorID,
		parentBlockTime,
		validators,
		placements,
		seed,
	}, nil
}

// Schedule to determine time of the proposer to produce a block, according to `nowTime`.
// `newBlockTime` is promised to be >= nowTime and > parentBlockTime
func (s *Scheduler) Schedule(nowTime uint64) (uint64, error) {
	const T = thor.BlockInterval

	newBlockTime := s.parentBlockTime + T
	if nowTime > newBlockTime {
		// ensure T aligned, and >= nowTime
		newBlockTime += (nowTime - newBlockTime + T - 1) / T * T
	}

	for i := range s.placements {
		slot := newBlockTime + uint64(i)*T
		_, addr := s.expectedValidator(slot)
		if addr == s.validator.Master {
			return slot, nil
		}
	}

	return 0, errors.Errorf("not scheduled within %d slots", len(s.placements))
}

// IsScheduled returns if the schedule(proposer, blockTime) is correct.
func (s *Scheduler) IsScheduled(blockTime uint64, proposer thor.Address) bool {
	if s.parentBlockTime >= blockTime {
		// invalid block time
		return false
	}

	T := thor.BlockInterval
	if (blockTime-s.parentBlockTime)%T != 0 {
		// invalid block time
		return false
	}

	_, addr := s.expectedValidator(blockTime)

	return addr == proposer
}

func (s *Scheduler) IsTheTime(newBlockTime uint64) bool {
	return s.IsScheduled(newBlockTime, s.validator.Master)
}

func (s *Scheduler) Updates(newBlockTime uint64) (map[thor.Bytes32]bool, uint64) {
	T := thor.BlockInterval

	updates := make(map[thor.Bytes32]bool)

	for i := uint64(0); i < uint64(len(s.placements)); i++ {
		if s.parentBlockTime+T+i*T >= newBlockTime {
			break
		}

		id, _ := s.expectedValidator(s.parentBlockTime + T + i*T)
		updates[id] = false
	}

	delete(updates, s.id) // we know the proposer is online
	score := uint64(len(s.placements) - len(updates))

	if !s.validator.Online {
		updates[s.id] = true
	}

	return updates, score
}

// expectedValidator returns the expected validator for the given block time.
// It uses the seed to deterministically select a validator.
func (s *Scheduler) expectedValidator(blockTime uint64) (thor.Bytes32, thor.Address) {
	hash := thor.Blake2b(s.seed, big.NewInt(0).SetUint64(blockTime).Bytes())
	selector := new(big.Rat).SetInt(new(big.Int).SetBytes(hash.Bytes()))
	divisor := new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), uint(len(hash)*8)))

	selector.Quo(selector, divisor)

	for i := range s.placements {
		if selector.Cmp(s.placements[i].start) >= 0 && selector.Cmp(s.placements[i].end) < 0 {
			return s.placements[i].id, s.placements[i].addr
		}
	}
	return thor.Bytes32{}, thor.Address{} // should never happen
}
