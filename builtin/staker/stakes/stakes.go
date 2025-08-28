// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package stakes

type WeightedStake struct {
	VET    uint64 // The amount of VET staked(in VET, not wei)
	Weight uint64 // The weight of the stake, calculated as (stake * multiplier / 100%)
}

func CalcWeight(vet uint64, multiplier uint8) uint64 {
	return vet * uint64(multiplier) / 100
}

func NewWeightedStakeWithMultiplier(vet uint64, multiplier uint8) *WeightedStake {
	return &WeightedStake{
		VET:    vet,
		Weight: CalcWeight(vet, multiplier),
	}
}

func NewWeightedStake(stake uint64, weight uint64) *WeightedStake {
	return &WeightedStake{
		VET:    stake,
		Weight: weight,
	}
}

func (s *WeightedStake) Add(new *WeightedStake) {
	s.VET += new.VET
	s.Weight += new.Weight
}

func (s *WeightedStake) Sub(new *WeightedStake) {
	s.VET -= new.VET
	s.Weight -= new.Weight
}
