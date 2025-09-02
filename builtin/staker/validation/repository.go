// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package validation

import (
	"encoding/binary"
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
	validations *solidity.Mapping[thor.Address, Validation]
	rewards     *solidity.Mapping[thor.Bytes32, *big.Int] // stores rewards per validator staking period

	exits *solidity.Mapping[thor.Bytes32, thor.Address] // exit block -> validator ID
}

func NewRepository(sctx *solidity.Context) *Repository {
	return &Repository{
		validations: solidity.NewMapping[thor.Address, Validation](sctx, slotValidations),
		rewards:     solidity.NewMapping[thor.Bytes32, *big.Int](sctx, slotRewards),
		exits:       solidity.NewMapping[thor.Bytes32, thor.Address](sctx, slotExitEpochs),
	}
}

func (r *Repository) getValidation(validator thor.Address) (*Validation, error) {
	v, err := r.validations.Get(validator)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator")
	}
	return &v, nil
}

func (r *Repository) setValidation(validator thor.Address, entry *Validation, isNew bool) error {
	if err := r.validations.Set(validator, *entry, isNew); err != nil {
		return errors.Wrap(err, "failed to set validator")
	}
	return nil
}

func (r *Repository) getReward(key thor.Bytes32) (*big.Int, error) {
	reward, err := r.rewards.Get(key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reward")
	}
	if reward == nil {
		return new(big.Int), nil
	}
	return reward, nil
}

func (r *Repository) setReward(key thor.Bytes32, val *big.Int, isNew bool) error {
	return r.rewards.Set(key, val, isNew)
}

func (r *Repository) getExit(block uint32) (thor.Address, error) {
	var key thor.Bytes32
	binary.BigEndian.PutUint32(key[:], block)

	return r.exits.Get(key)
}

func (r *Repository) setExit(block uint32, validator thor.Address) error {
	var key thor.Bytes32
	binary.BigEndian.PutUint32(key[:], block)

	if err := r.exits.Set(key, validator, true); err != nil {
		return errors.Wrap(err, "failed to set exit epoch")
	}
	return nil
}
