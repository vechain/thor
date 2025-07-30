// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package stakes

import "math/big"

type WeightedStake struct {
	vet        *big.Int // The amount of VET staked
	Multiplier uint8    // The multiplier applied to the stake
}

func NewWeightedStake(vet *big.Int, multiplier uint8) *WeightedStake {
	if vet == nil {
		vet = big.NewInt(0)
	}
	return &WeightedStake{
		vet:        vet,
		Multiplier: multiplier,
	}
}

func (s *WeightedStake) Weight() *big.Int {
	if s.vet == nil || s.vet.Sign() == 0 || s.Multiplier == 0 {
		return big.NewInt(0)
	}
	weight := new(big.Int).Mul(s.vet, big.NewInt(int64(s.Multiplier)))
	return weight.Div(weight, big.NewInt(100)) // weight = stake * multiplier / 100%
}

func (s *WeightedStake) VET() *big.Int {
	if s.vet == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(s.vet) // Return a copy to avoid external modification
}
