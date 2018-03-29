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
	baseGasPrice := builtin.Params.WithState(state).Get(thor.KeyBaseGasPrice)
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
func (r *ResolvedTransaction) BuyGas(blockTime uint64) (payer thor.Address, err error) {
	prepayedEnergy := new(big.Int).Mul(new(big.Int).SetUint64(r.tx.Gas()), r.GasPrice)
	payer, ok := builtin.Energy.WithState(r.state).Consume(blockTime, r.CommonTo, r.Origin, prepayedEnergy)
	if !ok {
		return thor.Address{}, errors.New("insufficient energy")
	}
	return payer, nil
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
