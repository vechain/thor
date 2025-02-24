// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pos

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/thor"
)

type Scheduler struct {
	validator       *staker.Validator
	addr            thor.Address
	parentBlockTime uint64
	shuffled        []thor.Address
	validators      map[thor.Address]*staker.Validator
}

// NewScheduler is a placeholder implementation for Staked based consensus.
// TODO: It will be replaced by a more sophisticated implementation in the future.
// It is currently based on the old PoA algorithm.
// https://github.com/vechain/protocol-board-repo/issues/429
func NewScheduler(
	signer thor.Address,
	validators map[thor.Address]*staker.Validator,
	parentBlockNumber uint32,
	parentBlockTime uint64,
	seed []byte,
) (*Scheduler, error) {
	var (
		listed    = false
		validator *staker.Validator
		list      []struct {
			addr thor.Address
			hash thor.Bytes32
		}
	)
	var num [4]byte
	binary.BigEndian.PutUint32(num[:], parentBlockNumber)

	for addr, entry := range validators {
		if addr == signer {
			validator = entry
			listed = true
		}

		list = append(list, struct {
			addr thor.Address
			hash thor.Bytes32
		}{
			addr,
			thor.Blake2b(seed, num[:], addr.Bytes()),
		})
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

	return &Scheduler{
		validator,
		signer,
		parentBlockTime,
		shuffled,
		validators,
	}, nil
}

func (s *Scheduler) Schedule(nowTime uint64) (newBlockTime uint64) {
	const T = thor.BlockInterval

	newBlockTime = s.parentBlockTime + T
	if nowTime > newBlockTime {
		// ensure T aligned, and >= nowTime
		newBlockTime += (nowTime - newBlockTime + T - 1) / T * T
	}

	offset := (newBlockTime-s.parentBlockTime)/T - 1
	for i, n := uint64(0), uint64(len(s.shuffled)); i < n; i++ {
		index := (i + offset) % n
		if s.shuffled[index] == s.addr {
			return newBlockTime + i*T
		}
	}

	// should never happen
	panic("something wrong with proposers list")
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

	index := (blockTime - s.parentBlockTime - T) / T % uint64(len(s.shuffled))
	return s.shuffled[index] == proposer
}

func (s *Scheduler) IsTheTime(newBlockTime uint64) bool {
	return s.IsScheduled(newBlockTime, s.addr)
}

func (s *Scheduler) Updates(newBlockTime uint64) ([]thor.Address, uint64) {
	T := thor.BlockInterval

	missed := make([]thor.Address, 0)

	for i := uint64(0); i < uint64(len(s.shuffled)); i++ {
		if s.parentBlockTime+T+i*T >= newBlockTime {
			break
		}
		missed = append(missed, s.shuffled[i])
	}

	// TODO: Slightly different behaviour to PoA, we don't reduce score for missed slots
	// https://github.com/vechain/protocol-board-repo/issues/433
	return missed, uint64(len(s.shuffled))
}
