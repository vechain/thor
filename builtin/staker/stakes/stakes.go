// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package stakes

import "math/big"

type WeightedStake struct {
	vet    *big.Int // The amount of VET staked
	weight *big.Int // The weight of the stake, calculated as (stake * multiplier / 100%)
}

func NewWeightedStake(vet *big.Int, multiplier uint8) *WeightedStake {
	weight := new(big.Int).Mul(vet, big.NewInt(int64(multiplier)))
	weight = weight.Div(weight, big.NewInt(100)) // weight = stake * multiplier / 100%
	return &WeightedStake{
		vet:    vet,
		weight: weight,
	}
}

func (s *WeightedStake) Weight() *big.Int {
	return s.weight
}

func (s *WeightedStake) VET() *big.Int {
	return s.vet
}
