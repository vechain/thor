// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package aggregation

import (
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/delta"
	"github.com/vechain/thor/v2/thor"
)

// Service manages delegation aggregations for each validator.
type Service struct {
	aggregationStorage *solidity.SimpleMapping[thor.Address, Aggregation]
}

func New(sctx *solidity.Context) *Service {
	return &Service{
		aggregationStorage: solidity.NewSimpleMapping[thor.Address, Aggregation](sctx, solidity.NumToSlot(15)),
	}
}

// GetAggregation retrieves the delegation aggregation for a validator.
// Returns a zero-initialized aggregation if none exists.
func (s *Service) GetAggregation(validator thor.Address) (*Aggregation, error) {
	d, err := s.aggregationStorage.Get(validator)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator aggregation")
	}

	if d.LockedVET == nil {
		return newAggregation(), nil
	}
	return &d, nil
}

// setAggregation stores the Aggregation
func (s *Service) setAggregation(validator thor.Address, agg *Aggregation, newValue bool) error {
	return s.aggregationStorage.Set(validator, *agg, newValue)
}

// Renew transitions the validator's delegations to the next staking period.
// Called during staking period renewal process.
func (s *Service) Renew(validator thor.Address) (*delta.Renewal, error) {
	agg, err := s.GetAggregation(validator)
	if err != nil {
		return nil, err
	}

	renew := agg.renew()

	if err = s.setAggregation(validator, agg, false); err != nil {
		return nil, err
	}

	return renew, nil
}

// Exit moves all delegations to withdrawable state when validator exits.
// Called when a validator is removed from the active set.
func (s *Service) Exit(validator thor.Address) (*delta.Exit, error) {
	agg, err := s.GetAggregation(validator)
	if err != nil {
		return nil, err
	}

	exit := agg.exit()

	if err = s.setAggregation(validator, agg, false); err != nil {
		return nil, err
	}

	return exit, nil
}
