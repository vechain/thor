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
	state        *state.State
	tx           *tx.Transaction
	Origin       thor.Address
	IntrinsicGas uint64
	BaseGasPrice *big.Int
	GasPrice     *big.Int
	Clauses      []*tx.Clause
	CommonTo     *thor.Address
}

// ResolveTransaction resolves the transaction and performs basic validation.
func ResolveTransaction(state *state.State, tx *tx.Transaction) (*ResolvedTransaction, error) {
	origin, err := tx.Signer()
	if err != nil {
		return nil, err
	}
	intrinsicGas, err := tx.IntrinsicGas()
	if err != nil {
		return nil, err
	}
	if intrinsicGas > tx.Gas() {
		return nil, errors.New("intrinsic gas exceeds provided gas")
	}
	baseGasPrice := builtin.Params.Native(state).Get(thor.KeyBaseGasPrice)
	gasPrice := tx.GasPrice(baseGasPrice)
	clauses := tx.Clauses()
	return &ResolvedTransaction{
		state,
		tx,
		origin,
		intrinsicGas,
		baseGasPrice,
		gasPrice,
		clauses,
		commonTo(clauses),
	}, nil
}

// BuyGas consumes energy to buy gas, to prepare for execution.
func (r *ResolvedTransaction) BuyGas(blockNum uint32) (payer thor.Address, prepaid *big.Int, returnGas func(uint64), err error) {
	energy := builtin.Energy.Native(r.state)
	doReturnGas := func(rgas uint64) *big.Int {
		returnedEnergy := new(big.Int).Mul(new(big.Int).SetUint64(rgas), r.GasPrice)
		energy.AddBalance(payer, returnedEnergy, blockNum)
		return returnedEnergy
	}

	prepaid = new(big.Int).Mul(new(big.Int).SetUint64(r.tx.Gas()), r.GasPrice)
	if r.CommonTo != nil {
		binding := builtin.Prototype.Native(r.state).Bind(*r.CommonTo)
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
					return sponsor, prepaid, doReturnGasAndSetCredit, nil
				}
			}
			// deduct from To
			if energy.SubBalance(*r.CommonTo, prepaid, blockNum) {
				return *r.CommonTo, prepaid, doReturnGasAndSetCredit, nil
			}
		}
	}

	// fallback to deduct from tx origin
	if energy.SubBalance(r.Origin, prepaid, blockNum) {
		return r.Origin, prepaid, func(rgas uint64) { doReturnGas(rgas) }, nil
	}
	return thor.Address{}, nil, nil, errors.New("insufficient energy")
}

// returns common 'To' field of clauses if any.
// Empty address returned if no common 'To'.
func commonTo(clauses []*tx.Clause) *thor.Address {
	if len(clauses) == 0 {
		return nil
	}

	firstTo := clauses[0].To()
	if firstTo == nil {
		return nil
	}

	for _, clause := range clauses[1:] {
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
