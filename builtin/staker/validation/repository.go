// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package validation

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/thor"
)

var (
	slotExitEpochs  = thor.BytesToBytes32([]byte(("exit-epochs")))
	slotValidations = thor.BytesToBytes32([]byte(("validations")))
	slotRewards     = thor.BytesToBytes32([]byte(("period-rewards")))
)

type Repository struct {
	validations *solidity.Mapping[thor.Address, *Validation]
	rewards     *solidity.Mapping[thor.Bytes32, *big.Int] // stores rewards per validator staking period

	exits *solidity.Mapping[*big.Int, thor.Address] // exit block -> validator ID
}

func NewRepository(sctx *solidity.Context) *Repository {
	return &Repository{
		validations: solidity.NewMapping[thor.Address, *Validation](sctx, slotValidations),
		rewards:     solidity.NewMapping[thor.Bytes32, *big.Int](sctx, slotRewards),
		exits:       solidity.NewMapping[*big.Int, thor.Address](sctx, slotExitEpochs),
	}
}

func (r *Repository) GetValidation(id thor.Address) (*Validation, error) {
	v, err := r.validations.Get(id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator")
	}
	return v, nil
}

func (r *Repository) SetValidation(id thor.Address, entry *Validation, isNew bool) error {
	if err := r.validations.Set(id, entry, isNew); err != nil {
		return errors.Wrap(err, "failed to set validator")
	}
	return nil
}

func (r *Repository) GetReward(key thor.Bytes32) (*big.Int, error) {
	return r.rewards.Get(key)
}

func (r *Repository) SetReward(key thor.Bytes32, val *big.Int, isNew bool) error {
	return r.rewards.Set(key, val, isNew)
}

func (r *Repository) GetExit(block *big.Int) (thor.Address, error) {
	return r.exits.Get(block)
}

func (r *Repository) SetExit(block uint32, id thor.Address) error {
	bigBlock := big.NewInt(0).SetUint64(uint64(block))

	if err := r.exits.Set(bigBlock, id, true); err != nil {
		return errors.Wrap(err, "failed to set exit epoch")
	}
	return nil
}
