package builtin

import (
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/state"
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

	energySharing struct {
		Credit       *big.Int
		RecoveryRate *big.Int
		Expiration   uint64
		Remained     *big.Int
		Timestamp    uint64
	}
)

var (
	_ state.StorageDecoder = (*stgProposer)(nil)
	_ state.StorageEncoder = (*stgProposer)(nil)

	_ state.StorageEncoder = (*energyAccount)(nil)
	_ state.StorageDecoder = (*energyAccount)(nil)

	_ state.StorageEncoder = (*energyGrowthRate)(nil)
	_ state.StorageDecoder = (*energyGrowthRate)(nil)

	_ state.StorageEncoder = (*energySharing)(nil)
	_ state.StorageDecoder = (*energySharing)(nil)
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

func (es *energySharing) Encode() ([]byte, error) {
	if es.Credit.Sign() == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(es)
}

func (es *energySharing) Decode(data []byte) error {
	if len(data) == 0 {
		*es = energySharing{&big.Int{}, &big.Int{}, 0, &big.Int{}, 0}
		return nil
	}
	return rlp.DecodeBytes(data, es)
}
func (es *energySharing) RemainedAt(blockTime uint64) *big.Int {
	if blockTime >= es.Expiration {
		return &big.Int{}
	}

	x := new(big.Int).SetUint64(blockTime - es.Timestamp)
	x.Mul(x, es.RecoveryRate)
	x.Add(x, es.Remained)
	if x.Cmp(es.Credit) < 0 {
		return x
	}
	return es.Credit
}
