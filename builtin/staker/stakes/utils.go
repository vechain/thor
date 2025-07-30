package stakes

import "math/big"

// WeightedStake calculates weight based on a stake and a multiplier
// TODO: Does this deserve its own package?
func WeightedStake(stake *big.Int, multiplier uint8) *big.Int {
	if multiplier == 0 {
		return big.NewInt(0)
	}
	weight := new(big.Int).Mul(stake, big.NewInt(int64(multiplier)))
	weight.Div(weight, big.NewInt(100)) // Convert percentage to weight
	return weight
}
