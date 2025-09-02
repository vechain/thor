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

type Storage struct {
	validations *solidity.Mapping[thor.Address, Validation]
	rewards     *solidity.Mapping[thor.Bytes32, *big.Int]
	exits       *solidity.Mapping[thor.Bytes32, thor.Address]
}

func NewStorage(sctx *solidity.Context) *Storage {
	return &Storage{
		validations: solidity.NewMapping[thor.Address, Validation](sctx, slotValidations),
		rewards:     solidity.NewMapping[thor.Bytes32, *big.Int](sctx, slotRewards),
		exits:       solidity.NewMapping[thor.Bytes32, thor.Address](sctx, slotExitEpochs),
	}
}

func (s *Storage) getValidation(validator thor.Address) (*Validation, error) {
	v, err := s.validations.Get(validator)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator")
	}
	return &v, nil
}

func (s *Storage) updateValidation(validator thor.Address, entry *Validation) error {
	if err := s.validations.Set(validator, *entry, false); err != nil {
		return errors.Wrap(err, "failed to set validator")
	}
	return nil
}

func (s *Storage) setValidation(validator thor.Address, entry Validation, isNew bool) error {
	if err := s.validations.Set(validator, entry, isNew); err != nil {
		return errors.Wrap(err, "failed to set validator")
	}
	return nil
}

func (s *Storage) getReward(key thor.Bytes32) (*big.Int, error) {
	reward, err := s.rewards.Get(key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reward")
	}
	if reward == nil {
		return new(big.Int), nil
	}
	return reward, nil
}

func (s *Storage) setReward(key thor.Bytes32, val *big.Int, isNew bool) error {
	return s.rewards.Set(key, val, isNew)
}

func (s *Storage) getExit(block uint32) (thor.Address, error) {
	var key thor.Bytes32
	binary.BigEndian.PutUint32(key[:], block)

	return s.exits.Get(key)
}

func (s *Storage) setExit(block uint32, validator thor.Address) error {
	var key thor.Bytes32
	binary.BigEndian.PutUint32(key[:], block)

	if err := s.exits.Set(key, validator, true); err != nil {
		return errors.Wrap(err, "failed to set exit epoch")
	}
	return nil
}
