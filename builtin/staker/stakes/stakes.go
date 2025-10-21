// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package stakes

import (
	"errors"

	"github.com/ethereum/go-ethereum/common/math"
)

type WeightedStake struct {
	VET    uint64 // The amount of VET staked(in VET, not wei)
	Weight uint64 // The weight of the stake, calculated as (stake * multiplier / 100%)
}

func calcWeight(vet uint64, multiplier uint8) uint64 {
	return vet * uint64(multiplier) / 100
}

func NewWeightedStakeWithMultiplier(vet uint64, multiplier uint8) *WeightedStake {
	return &WeightedStake{
		VET:    vet,
		Weight: calcWeight(vet, multiplier),
	}
}

func NewWeightedStake(stake uint64, weight uint64) *WeightedStake {
	return &WeightedStake{
		VET:    stake,
		Weight: weight,
	}
}

func (s *WeightedStake) Add(new *WeightedStake) error {
	var overflow bool

	if s.VET, overflow = math.SafeAdd(s.VET, new.VET); overflow {
		return errors.New("VET add overflow occurred")
	}

	if s.Weight, overflow = math.SafeAdd(s.Weight, new.Weight); overflow {
		return errors.New("weight add overflow occurred")
	}

	return nil
}

func (s *WeightedStake) Sub(new *WeightedStake) error {
	var underflow bool

	if s.VET, underflow = math.SafeSub(s.VET, new.VET); underflow {
		return errors.New("VET subtract underflow occurred")
	}

	if s.Weight, underflow = math.SafeSub(s.Weight, new.Weight); underflow {
		return errors.New("weight subtract underflow occurred")
	}

	return nil
}

func (s *WeightedStake) Clone() *WeightedStake {
	return &WeightedStake{
		VET:    s.VET,
		Weight: s.Weight,
	}
}
