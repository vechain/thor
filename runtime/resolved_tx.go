// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package runtime

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/pkg/errors"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/xenv"
)

// ResolvedTransaction resolve the transaction according to given state.
type ResolvedTransaction struct {
	tx           *tx.Transaction
	Origin       thor.Address
	Delegator    *thor.Address
	IntrinsicGas uint64
	Clauses      []*tx.Clause
}

// ResolveTransaction resolves the transaction and performs basic validation.
func ResolveTransaction(tx *tx.Transaction) (*ResolvedTransaction, error) {
	origin, err := tx.Origin()
	if err != nil {
		return nil, err
	}
	intrinsicGas, err := tx.IntrinsicGas()
	if err != nil {
		return nil, err
	}
	if tx.Gas() < intrinsicGas {
		return nil, errors.New("intrinsic gas exceeds provided gas")
	}
	delegator, err := tx.Delegator()
	if err != nil {
		return nil, err
	}

	clauses := tx.Clauses()
	sumValue := new(big.Int)
	for _, clause := range clauses {
		value := clause.Value()
		if value.Sign() < 0 {
			return nil, errors.New("clause with negative value")
		}

		sumValue.Add(sumValue, value)
		if sumValue.Cmp(math.MaxBig256) > 0 {
			return nil, errors.New("tx value too large")
		}
	}

	return &ResolvedTransaction{
		tx,
		origin,
		delegator,
		intrinsicGas,
		clauses,
	}, nil
}

// CommonTo returns common 'To' field of clauses if any.
// Nil returned if no common 'To'.
func (r *ResolvedTransaction) CommonTo() *thor.Address {
	if len(r.Clauses) == 0 {
		return nil
	}

	firstTo := r.Clauses[0].To()
	if firstTo == nil {
		return nil
	}

	for _, clause := range r.Clauses[1:] {
		to := clause.To()
		if to == nil {
			return nil
		}
		if *to != *firstTo {
			return nil
		}
	}
	return firstTo
}

// BuyGas consumes energy to buy gas, to prepare for execution.
func (r *ResolvedTransaction) BuyGas(state *state.State, blockTime uint64) (
	baseGasPrice *big.Int,
	gasPrice *big.Int,
	payer thor.Address,
	returnGas func(uint64) error,
	err error,
) {
	if baseGasPrice, err = builtin.Params.Native(state).Get(thor.KeyBaseGasPrice); err != nil {
		return
	}
	gasPrice = r.tx.GasPrice(baseGasPrice)

	energy := builtin.Energy.Native(state, blockTime)
	doReturnGas := func(rgas uint64) (*big.Int, error) {
		returnedEnergy := new(big.Int).Mul(new(big.Int).SetUint64(rgas), gasPrice)
		if err := energy.Add(payer, returnedEnergy); err != nil {
			return nil, err
		}
		return returnedEnergy, nil
	}

	prepaid := new(big.Int).Mul(new(big.Int).SetUint64(r.tx.Gas()), gasPrice)
	if r.Delegator != nil {
		var sufficient bool
		if sufficient, err = energy.Sub(*r.Delegator, prepaid); err != nil {
			return
		}
		if sufficient {
			return baseGasPrice, gasPrice, *r.Delegator, func(rgas uint64) error {
				_, err := doReturnGas(rgas)
				return err
			}, nil
		}
		return nil, nil, thor.Address{}, nil, errors.New("insufficient energy")
	}

	commonTo := r.CommonTo()
	if commonTo != nil {
		binding := builtin.Prototype.Native(state).Bind(*commonTo)
		var credit *big.Int
		if credit, err = binding.UserCredit(r.Origin, blockTime); err != nil {
			return
		}
		if credit.Cmp(prepaid) >= 0 {
			doReturnGasAndSetCredit := func(rgas uint64) error {
				returnedEnergy, err := doReturnGas(rgas)
				if err != nil {
					return err
				}

				usedEnergy := new(big.Int).Sub(prepaid, returnedEnergy)
				return binding.SetUserCredit(r.Origin, new(big.Int).Sub(credit, usedEnergy), blockTime)
			}
			var sponsor thor.Address
			if sponsor, err = binding.CurrentSponsor(); err != nil {
				return
			}

			var isSponsor bool
			if isSponsor, err = binding.IsSponsor(sponsor); err != nil {
				return
			}

			// has enough credit
			if isSponsor {
				// deduct from sponsor, if any
				var ok bool
				ok, err = energy.Sub(sponsor, prepaid)
				if err != nil {
					return
				}
				if ok {
					return baseGasPrice, gasPrice, sponsor, doReturnGasAndSetCredit, nil
				}
			}
			// deduct from To
			var sufficient bool
			sufficient, err = energy.Sub(*commonTo, prepaid)
			if err != nil {
				return
			}
			if sufficient {
				return baseGasPrice, gasPrice, *commonTo, doReturnGasAndSetCredit, nil
			}
		}
	}

	// fallback to deduct from tx origin
	var sufficient bool
	if sufficient, err = energy.Sub(r.Origin, prepaid); err != nil {
		return
	}

	if sufficient {
		return baseGasPrice, gasPrice, r.Origin, func(rgas uint64) error { _, err := doReturnGas(rgas); return err }, nil
	}
	return nil, nil, thor.Address{}, nil, errors.New("insufficient energy")
}

// ToContext create a tx context object.
func (r *ResolvedTransaction) ToContext(
	gasPrice *big.Int,
	gasPayer thor.Address,
	blockNumber uint32,
	getBlockID func(uint32) (thor.Bytes32, error),
) (*xenv.TransactionContext, error) {
	provedWork, err := r.tx.ProvedWork(blockNumber, getBlockID)
	if err != nil {
		return nil, err
	}
	return &xenv.TransactionContext{
		ID:         r.tx.ID(),
		Origin:     r.Origin,
		GasPayer:   gasPayer,
		GasPrice:   gasPrice,
		ProvedWork: provedWork,
		BlockRef:   r.tx.BlockRef(),
		Expiration: r.tx.Expiration(),
	}, nil
}
