package thor

import "math/big"

type provedWork struct {
	baseEnergyExchangeRate *big.Int
	baseTimestamp          uint64
}

var (
	// ProvedWork obj to convert proved work to energy.
	ProvedWork = &provedWork{
		big.NewInt(1e18), //TODO to be determined
		1519315348,
	}
	big100 = big.NewInt(100)
	big104 = big.NewInt(104) // Moore's law monthly rate (percentage)
	bigE18 = big.NewInt(1e18)
)

// Energy exchange proved work to energy.
// The decay curve follows Moore's law.
func (pw *provedWork) ToEnergy(work *big.Int, timestamp uint64) *big.Int {
	// months past from baseWorkProofTimestamp to timestamp
	var months *big.Int
	if timestamp > pw.baseTimestamp {
		months = new(big.Int).SetUint64((timestamp - pw.baseTimestamp) / 3600 / 24 / 30)
	} else {
		months = &big.Int{}
	}

	energy := &big.Int{}
	energy.Mul(work, pw.baseEnergyExchangeRate)
	energy.Div(energy, bigE18)

	x := &big.Int{}

	if months.Sign() != 0 {
		energy.Mul(energy, x.Exp(big100, months, nil))
		energy.Div(energy, x.Exp(big104, months, nil))
	}
	return energy
}
