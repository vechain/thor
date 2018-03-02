package builtin

import (
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

type (
	stgProposer poa.Proposer

	energyAccount struct {
		Balance *big.Int

		// snapshot
		Timestamp    uint64
		TokenBalance *big.Int
	}

	energyGrowthRate struct {
		Rate      *big.Int
		Timestamp uint64
	}

	energyConsumptionApproval struct {
		Credit       *big.Int
		RecoveryRate *big.Int
		Expiration   uint64
		Remained     *big.Int
		Timestamp    uint64
	}

	energySupplier struct {
		Address thor.Address
		Agreed  bool
	}
)

var (
	_ state.StorageDecoder = (*stgProposer)(nil)
	_ state.StorageEncoder = (*stgProposer)(nil)

	_ state.StorageEncoder = (*energyAccount)(nil)
	_ state.StorageDecoder = (*energyAccount)(nil)

	_ state.StorageEncoder = (*energyGrowthRate)(nil)
	_ state.StorageDecoder = (*energyGrowthRate)(nil)

	_ state.StorageEncoder = (*energyConsumptionApproval)(nil)
	_ state.StorageDecoder = (*energyConsumptionApproval)(nil)

	_ state.StorageEncoder = (*energySupplier)(nil)
	_ state.StorageDecoder = (*energySupplier)(nil)
)

func (s *stgProposer) Encode() ([]byte, error) {
	if s.Address.IsZero() && s.Status == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(s)
}

func (s *stgProposer) Decode(data []byte) error {
	if len(data) == 0 {
		*s = stgProposer{}
		return nil
	}
	return rlp.DecodeBytes(data, s)
}

/////

func (ea *energyAccount) Encode() ([]byte, error) {
	if ea.Balance.Sign() == 0 &&
		ea.Timestamp == 0 &&
		ea.TokenBalance.Sign() == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(ea)
}

func (ea *energyAccount) Decode(data []byte) error {
	if len(data) == 0 {
		*ea = energyAccount{&big.Int{}, 0, &big.Int{}}
		return nil
	}
	return rlp.DecodeBytes(data, ea)
}

////

func (egr *energyGrowthRate) Encode() ([]byte, error) {
	if egr.Rate.Sign() == 0 && egr.Timestamp == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(egr)
}

func (egr *energyGrowthRate) Decode(data []byte) error {
	if len(data) == 0 {
		*egr = energyGrowthRate{&big.Int{}, 0}
		return nil
	}
	return rlp.DecodeBytes(data, egr)
}

///

func (eca *energyConsumptionApproval) Encode() ([]byte, error) {
	if eca.Credit.Sign() == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(eca)
}

func (eca *energyConsumptionApproval) Decode(data []byte) error {
	if len(data) == 0 {
		*eca = energyConsumptionApproval{&big.Int{}, &big.Int{}, 0, &big.Int{}, 0}
		return nil
	}
	return rlp.DecodeBytes(data, eca)
}

func (eca *energyConsumptionApproval) RemainedAt(blockTime uint64) *big.Int {
	if blockTime >= eca.Expiration {
		return &big.Int{}
	}

	x := new(big.Int).SetUint64(blockTime - eca.Timestamp)
	x.Mul(x, eca.RecoveryRate)
	x.Add(x, eca.Remained)
	if x.Cmp(eca.Credit) < 0 {
		return x
	}
	return eca.Credit
}

///
func (es *energySupplier) Encode() ([]byte, error) {
	if !es.Agreed && es.Address.IsZero() {
		return nil, nil
	}
	return rlp.EncodeToBytes(es)
}

func (es *energySupplier) Decode(data []byte) error {
	if len(data) == 0 {
		*es = energySupplier{}
		return nil
	}
	return rlp.DecodeBytes(data, es)
}
