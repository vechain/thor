package energy

import (
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var bigE18 = big.NewInt(1e18)

type (
	account struct {
		Balance *big.Int

		// snapshot
		Timestamp    uint64
		TokenBalance *big.Int
	}

	growthRate struct {
		Rate      *big.Int
		Timestamp uint64
	}

	consumptionApproval struct {
		Credit       *big.Int
		RecoveryRate *big.Int
		Expiration   uint64
		Remained     *big.Int
		Timestamp    uint64
	}

	supplier struct {
		Address thor.Address
		Agreed  bool
	}
)

var (
	_ state.StorageEncoder = (*account)(nil)
	_ state.StorageDecoder = (*account)(nil)

	_ state.StorageEncoder = (growthRates)(nil)
	_ state.StorageDecoder = (*growthRates)(nil)

	_ state.StorageEncoder = (*consumptionApproval)(nil)
	_ state.StorageDecoder = (*consumptionApproval)(nil)

	_ state.StorageEncoder = (*supplier)(nil)
	_ state.StorageDecoder = (*supplier)(nil)
)

func (a *account) Encode() ([]byte, error) {
	if a.Balance.Sign() == 0 &&
		a.Timestamp == 0 &&
		a.TokenBalance.Sign() == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(a)
}

func (a *account) Decode(data []byte) error {
	if len(data) == 0 {
		*a = account{&big.Int{}, 0, &big.Int{}}
		return nil
	}
	return rlp.DecodeBytes(data, a)
}

func (a *account) CalcBalance(blockTime uint64, rates []*growthRate) *big.Int {
	if a.Timestamp >= blockTime {
		// never occur in real env.
		return a.Balance
	}

	if a.TokenBalance.Sign() == 0 {
		return a.Balance
	}

	rateCount := len(rates)

	t2 := blockTime
	newBalance := new(big.Int).Set(a.Balance)

	// reversedly iterates rates
	for i := rateCount; i > 0; i-- {
		rate := rates[i-1]

		t1 := rate.Timestamp
		if t1 < a.Timestamp {
			t1 = a.Timestamp
		}

		if t1 > t2 {
			// never occur in real env.
			return a.Balance
		}

		if t1 != t2 && a.TokenBalance.Sign() != 0 && rate.Rate.Sign() != 0 {
			// energy growth (token * rate * dt / 1e18)
			x := new(big.Int).SetUint64(t2 - t1)
			x.Mul(x, rate.Rate)
			x.Mul(x, a.TokenBalance)
			x.Div(x, bigE18)
			newBalance.Add(newBalance, x)
		}

		t2 = rate.Timestamp

		if a.Timestamp >= rate.Timestamp {
			break
		}
	}
	return newBalance
}

////

type growthRates []*growthRate

func (grs growthRates) Encode() ([]byte, error) {
	if len(grs) == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(grs)
}

func (grs *growthRates) Decode(data []byte) error {
	if len(data) == 0 {
		*grs = nil
		return nil
	}
	return rlp.DecodeBytes(data, grs)
}

///

func (ca *consumptionApproval) Encode() ([]byte, error) {
	if ca.Credit.Sign() == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(ca)
}

func (ca *consumptionApproval) Decode(data []byte) error {
	if len(data) == 0 {
		*ca = consumptionApproval{&big.Int{}, &big.Int{}, 0, &big.Int{}, 0}
		return nil
	}
	return rlp.DecodeBytes(data, ca)
}

func (ca *consumptionApproval) RemainedAt(blockTime uint64) *big.Int {
	if blockTime >= ca.Expiration {
		return &big.Int{}
	}

	x := new(big.Int).SetUint64(blockTime - ca.Timestamp)
	x.Mul(x, ca.RecoveryRate)
	x.Add(x, ca.Remained)
	if x.Cmp(ca.Credit) < 0 {
		return x
	}
	return ca.Credit
}

///
func (s *supplier) Encode() ([]byte, error) {
	if !s.Agreed && s.Address.IsZero() {
		return nil, nil
	}
	return rlp.EncodeToBytes(s)
}

func (s *supplier) Decode(data []byte) error {
	if len(data) == 0 {
		*s = supplier{}
		return nil
	}
	return rlp.DecodeBytes(data, s)
}
