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

	maxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
)

type Service struct {
	delegations *solidity.Mapping[*big.Int, *body]
	idCounter   *solidity.Raw[*big.Int]
}

func New(sctx *solidity.Context) *Service {
	return &Service{
		delegations: solidity.NewMapping[*big.Int, *body](sctx, slotDelegations),
		idCounter:   solidity.NewRaw[*big.Int](sctx, slotDelegationsCounter),
	}
}

// GetDelegation retrieves the delegation for a validator.
func (s *Service) GetDelegation(delegationID *big.Int) (*Delegation, error) {
	b, err := s.delegations.Get(delegationID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get delegation")
	}
	if b == nil {
		return nil, nil
	}

	return &Delegation{b}, nil
}

func (s *Service) Add(
	validator thor.Address,
	firstIteration uint32,
	stake uint64,
	multiplier uint8,
) (*big.Int, error) {
	id, err := s.newDelegationID()
	if err != nil {
		return nil, err
	}

	delegationID := new(big.Int).Set(id)
	delegation := &Delegation{
		&body{
			Validation:     validator,
			Multiplier:     multiplier,
			Stake:          stake,
			FirstIteration: firstIteration,
		},
	}

	if err := s.delegations.Insert(delegationID, delegation.body); err != nil {
		return nil, errors.Wrap(err, "failed to set delegation")
	}

	return delegationID, nil
}

func (s *Service) SignalExit(delegation *Delegation, delegationID *big.Int, valCurrentIteration uint32) error {
	delegation.body.LastIteration = &valCurrentIteration

	return s.delegations.Update(delegationID, delegation.body)
}

func (s *Service) Withdraw(del *Delegation, delegationID *big.Int) (uint64, error) {
	// ensure the pointers are copied, not referenced
	withdrawableStake := del.Stake()

	del.body.Stake = 0
	if err := s.delegations.Update(delegationID, del.body); err != nil {
		return 0, err
	}

	return withdrawableStake, nil
}

func (s *Service) newDelegationID() (*big.Int, error) {
	// update the global delegation counter
	id, err := s.idCounter.Get()
	if err != nil {
		return nil, err
	}
	// first seen will be a nil pointer
	if id == nil {
		id = big.NewInt(0)
	}

	id.Add(id, big.NewInt(1))
	if id.Cmp(maxUint256) >= 0 {
		return nil, errors.New("delegation ID counter overflow: maximum delegations reached")
	}
	if err := s.idCounter.Upsert(id); err != nil {
		return nil, err
	}
	return id, nil
}
