// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package delegation

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/reverts"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
)

var (
	slotDelegations        = thor.BytesToBytes32([]byte(("delegations")))
	slotDelegationsCounter = thor.BytesToBytes32([]byte(("delegations-counter")))
)

type Service struct {
	delegations *solidity.Mapping[*big.Int, Delegation]
	idCounter   *solidity.Uint256
}

func New(sctx *solidity.Context) *Service {
	return &Service{
		delegations: solidity.NewMapping[*big.Int, Delegation](sctx, slotDelegations),
		idCounter:   solidity.NewUint256(sctx, slotDelegationsCounter),
	}
}

func (s *Service) GetDelegation(delegationID *big.Int) (*Delegation, error) {
	d, err := s.delegations.Get(delegationID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get delegation")
	}
	return &d, nil
}

func (s *Service) setDelegation(delegationID *big.Int, entry *Delegation, isNew bool) error {
	if err := s.delegations.Set(delegationID, *entry, isNew); err != nil {
		return errors.Wrap(err, "failed to set delegation")
	}
	return nil
}

func (s *Service) Add(
	validator thor.Address,
	firstIteration uint32,
	stake uint64,
	multiplier uint8,
) (*big.Int, error) {
	// ensure input is sane
	if multiplier == 0 {
		return nil, reverts.New("multiplier cannot be 0")
	}

	// update the global delegation counter
	id, err := s.idCounter.Get()
	if err != nil {
		return nil, err
	}

	id = id.Add(id, big.NewInt(1))
	if err := s.idCounter.Set(id); err != nil {
		return nil, errors.Wrap(err, "failed to increment delegation ID counter")
	}

	delegationID := new(big.Int).Set(id)
	delegation := Delegation{
		Validation:     validator,
		Multiplier:     multiplier,
		Stake:          stake,
		FirstIteration: firstIteration,
	}

	if err := s.delegations.Set(delegationID, delegation, false); err != nil {
		return nil, errors.Wrap(err, "failed to set delegation")
	}

	return delegationID, nil
}

func (s *Service) SignalExit(delegation *Delegation, delegationID *big.Int, valCurrentIteration uint32) error {
	if delegation.LastIteration != nil {
		return reverts.New("delegation is already disabled for auto-renew")
	}
	if delegation.Stake == 0 {
		return reverts.New("delegation is not active")
	}

	delegation.LastIteration = &valCurrentIteration

	return s.setDelegation(delegationID, delegation, false)
}

func (s *Service) Withdraw(del *Delegation, delegationID *big.Int, val *validation.Validation) (uint64, error) {
	// ensure the pointers are copied, not referenced
	withdrawableStake := del.Stake

	del.Stake = 0
	if err := s.setDelegation(delegationID, del, false); err != nil {
		return 0, err
	}

	return withdrawableStake, nil
}
