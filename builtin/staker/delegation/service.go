// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package delegation

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/solidity"
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

func (s *Service) SetDelegation(delegationID *big.Int, entry *Delegation, isNew bool) error {
	if err := s.delegations.Set(delegationID, *entry, isNew); err != nil {
		return errors.Wrap(err, "failed to set delegation")
	}
	return nil
}

func (s *Service) Add(
	validator thor.Address,
	firstIteration uint32,
	stake *big.Int,
	multiplier uint8,
) (*big.Int, error) {
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
		Validator:      validator,
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
	delegation.LastIteration = &valCurrentIteration

	return s.SetDelegation(delegationID, delegation, false)
}

func (s *Service) Withdraw(del *Delegation, delegationID *big.Int) (*big.Int, error) {
	// ensure the pointers are copied, not referenced
	withdrawableStake := new(big.Int).Set(del.Stake)

	del.Stake = big.NewInt(0)
	if err := s.SetDelegation(delegationID, del, false); err != nil {
		return nil, err
	}

	return withdrawableStake, nil
}
