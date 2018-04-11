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
		BlockNum uint32
	}

	consumptionApproval struct {
		Credit       *big.Int
		RecoveryRate *big.Int
		Expiration   uint32
		Remained     *big.Int
		BlockNum     uint32
	}

	supplier struct {
		Address thor.Address
		Agreed  bool
	}
)

var (
	_ state.StorageEncoder = (*account)(nil)
	_ state.StorageDecoder = (*account)(nil)

	_ state.StorageEncoder = (*consumptionApproval)(nil)
	_ state.StorageDecoder = (*consumptionApproval)(nil)

	_ state.StorageEncoder = (*supplier)(nil)
	_ state.StorageDecoder = (*supplier)(nil)
)

func (a *account) Encode() ([]byte, error) {
	if a.Balance.Sign() == 0 &&
		a.BlockNum == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(a)
}

func (a *account) Decode(data []byte) error {
	if len(data) == 0 {
		*a = account{&big.Int{}, 0}
		return nil
	}
	return rlp.DecodeBytes(data, a)
}

func (a *account) CalcBalance(tokenBalance *big.Int, blockNum uint32) *big.Int {
	if blockNum <= a.BlockNum {
		// never occur in real env.
		return a.Balance
	}

	if tokenBalance.Sign() == 0 {
		return a.Balance
	}

	t := blockNum - a.BlockNum
	if t == 0 {
		return a.Balance
	}
	x := new(big.Int).SetUint64(uint64(t))
	x.Mul(x, tokenBalance)
	x.Mul(x, thor.EnergyGrowthRate)
	x.Div(x, bigE18)
	return new(big.Int).Add(a.Balance, x)
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

func (ca *consumptionApproval) RemainedAt(blockNum uint32) *big.Int {
	if blockNum >= ca.Expiration {
		return &big.Int{}
	}

	x := new(big.Int).SetUint64(uint64(blockNum - ca.BlockNum))
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
