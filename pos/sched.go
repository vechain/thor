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
	validator       *staker.Validator
	addr            thor.Address
	parentBlockTime uint64
	validators      map[thor.Address]*staker.Validator
	placements      []placement
	seed            []byte
}

type placement struct {
	start *big.Rat
	end   *big.Rat
	addr  thor.Address
	hash  thor.Bytes32
}

// NewScheduler is a placeholder implementation for Staked based consensus.
// TODO: It will be replaced by a more sophisticated implementation in the future.
// It is currently based on the old PoA algorithm.
// https://github.com/vechain/protocol-board-repo/issues/429
func NewScheduler(
	signer thor.Address,
	validators map[thor.Address]*staker.Validator,
	totalStake *big.Int,
	parentBlockNumber uint32,
	parentBlockTime uint64,
	seed []byte,
) (*Scheduler, error) {
	if len(validators) == 0 {
		return nil, errors.New("no validators")
	}
	if totalStake.Sign() == 0 {
		return nil, errors.New("total stake is zero")
	}
	var (
		listed    = false
		validator *staker.Validator
	)
	var num [4]byte
	binary.BigEndian.PutUint32(num[:], parentBlockNumber)

	placements := make([]placement, 0, len(validators))

	for addr, entry := range validators {
		if addr == signer {
			validator = entry
			listed = true
		}

		placements = append(placements, placement{
			addr: addr,
			hash: thor.Blake2b(seed, num[:], addr.Bytes()),
		})
	}

	if !listed {
		return nil, errors.New("unauthorized block proposer")
	}

	sort.Slice(placements, func(i, j int) bool {
		return bytes.Compare(placements[i].hash.Bytes(), placements[j].hash.Bytes()) < 0
	})

	prev := big.NewRat(0, 1)
	totalStakeRat := new(big.Rat).SetInt(totalStake)

	for i := range placements {
		weightRat := new(big.Rat).SetInt(validators[placements[i].addr].Weight)
		weight := new(big.Rat).Quo(weightRat, totalStakeRat)

		placements[i].start = new(big.Rat).Set(prev)
		placements[i].end = new(big.Rat).Add(prev, weight)
		prev = placements[i].end
	}

	return &Scheduler{
		validator,
		signer,
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

	for i := 0; i < len(s.placements); i++ {
		slot := newBlockTime + uint64(i)*T
		if s.expectedValidator(slot) == s.addr {
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

	return s.expectedValidator(blockTime) == proposer
}

func (s *Scheduler) IsTheTime(newBlockTime uint64) bool {
	return s.IsScheduled(newBlockTime, s.addr)
}

func (s *Scheduler) Updates(newBlockTime uint64) ([]thor.Address, uint64) {
	T := thor.BlockInterval

	missed := make([]thor.Address, 0)

	for i := uint64(0); i < uint64(len(s.placements)); i++ {
		if s.parentBlockTime+T+i*T >= newBlockTime {
			break
		}

		addr := s.expectedValidator(s.parentBlockTime + T + i*T)
		missed = append(missed, addr)
	}

	// TODO: Slightly different behaviour to PoA, we don't reduce score for missed slots
	// https://github.com/vechain/protocol-board-repo/issues/433
	return missed, uint64(len(s.placements))
}

// expectedValidator returns the expected validator for the given block time.
// It uses the seed to deterministically select a validator.
func (s *Scheduler) expectedValidator(blockTime uint64) thor.Address {
	hash := thor.Blake2b(s.seed, big.NewInt(0).SetUint64(blockTime).Bytes())
	selector := new(big.Rat).SetInt(new(big.Int).SetBytes(hash.Bytes()))
	divisor := new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), uint(len(hash)*8)))

	selector.Quo(selector, divisor)

	for i := range s.placements {
		if selector.Cmp(s.placements[i].start) >= 0 && selector.Cmp(s.placements[i].end) < 0 {
			return s.placements[i].addr
		}
	}
	return thor.Address{}
}
