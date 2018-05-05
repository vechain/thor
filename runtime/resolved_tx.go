package runtime

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// ResolvedTransaction resolve the transaction according to given state.
type ResolvedTransaction struct {
	state *state.State
	tx    *tx.Transaction

	Origin       thor.Address
	BaseGasPrice *big.Int
	GasPrice     *big.Int
	Clauses      []*tx.Clause
}

// ResolveTransaction resolves the transaction and performs basic validation.
func ResolveTransaction(state *state.State, tx *tx.Transaction) (*ResolvedTransaction, error) {
	origin, err := tx.Signer()
	if err != nil {
		return nil, err
	}

	baseGasPrice := builtin.Params.Native(state).Get(thor.KeyBaseGasPrice)
	gasPrice := tx.GasPrice(baseGasPrice)
	clauses := tx.Clauses()
	return &ResolvedTransaction{
		state,
		tx,
		origin,
		baseGasPrice,
		gasPrice,
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
func (r *ResolvedTransaction) BuyGas(blockNum uint32) (payer thor.Address, leftOverGas uint64, returnGas func(uint64), err error) {
	intrinsicGas, err := r.tx.IntrinsicGas()
	if err != nil {
		return thor.Address{}, 0, nil, err
	}
	if intrinsicGas > r.tx.Gas() {
		return thor.Address{}, 0, nil, errors.New("intrinsic gas exceeds provided gas")
	}

	leftOverGas = r.tx.Gas() - intrinsicGas

	energy := builtin.Energy.Native(r.state)
	doReturnGas := func(rgas uint64) *big.Int {
		returnedEnergy := new(big.Int).Mul(new(big.Int).SetUint64(rgas), r.GasPrice)
		energy.AddBalance(payer, returnedEnergy, blockNum)
		return returnedEnergy
	}

	prepaid := new(big.Int).Mul(new(big.Int).SetUint64(r.tx.Gas()), r.GasPrice)
	commonTo := r.CommonTo()
	if commonTo != nil {
		binding := builtin.Prototype.Native(r.state).Bind(*commonTo)
		credit := binding.UserCredit(r.Origin, blockNum)
		if credit.Cmp(prepaid) >= 0 {
			doReturnGasAndSetCredit := func(rgas uint64) {
				returnedEnergy := doReturnGas(rgas)
				usedEnergy := new(big.Int).Sub(prepaid, returnedEnergy)
				binding.SetUserCredit(r.Origin, new(big.Int).Sub(credit, usedEnergy), blockNum)
			}
			// has enough credit
			if sponsor := binding.CurrentSponsor(); !sponsor.IsZero() {
				// deduct from sponsor, if any
				if energy.SubBalance(sponsor, prepaid, blockNum) {
					return sponsor, leftOverGas, doReturnGasAndSetCredit, nil
				}
			}
			// deduct from To
			if energy.SubBalance(*commonTo, prepaid, blockNum) {
				return *commonTo, leftOverGas, doReturnGasAndSetCredit, nil
			}
		}
	}

	// fallback to deduct from tx origin
	if energy.SubBalance(r.Origin, prepaid, blockNum) {
		return r.Origin, leftOverGas, func(rgas uint64) { doReturnGas(rgas) }, nil
	}
	return thor.Address{}, 0, nil, errors.New("insufficient energy")
}
