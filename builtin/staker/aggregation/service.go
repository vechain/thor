// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package aggregation

import (
	"fmt"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/globalstats"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
	"github.com/vechain/thor/v2/thor"
)

var slotAggregations = thor.BytesToBytes32([]byte("aggregated-delegations"))

// Service manages delegation aggregations for each validator.
type Service struct {
	aggregationStorage *solidity.Mapping[thor.Address, *Aggregation]
}

func New(sctx *solidity.Context) *Service {
	return &Service{
		aggregationStorage: solidity.NewMapping[thor.Address, *Aggregation](sctx, slotAggregations),
	}
}

// GetAggregation retrieves the delegation aggregation for a validator.
// Returns a zero-initialized aggregation if none exists.
func (s *Service) GetAggregation(validator thor.Address) (*Aggregation, error) {
	agg, err := s.aggregationStorage.Get(validator)
	if err != nil {
		return nil, fmt.Errorf("failed to get validator aggregation: %w", err)
	}

	if agg == nil {
		return newAggregation(), nil
	}
	return agg, nil
}

// AddPendingVET adds a new delegation to the validator's pending pool.
// Called when a delegator creates a new delegation.
func (s *Service) AddPendingVET(validator thor.Address, stake *stakes.WeightedStake) error {
	agg, err := s.GetAggregation(validator)
	if err != nil {
		return err
	}

	if err = agg.Pending.Add(stake); err != nil {
		return err
	}

	return s.aggregationStorage.Upsert(validator, agg)
}

// SubPendingVet removes VET from the validator's pending pool.
// Called when a delegation is withdrawn before becoming active.
func (s *Service) SubPendingVet(validator thor.Address, stake *stakes.WeightedStake) error {
	agg, err := s.GetAggregation(validator)
	if err != nil {
		return err
	}
	if err = agg.Pending.Sub(stake); err != nil {
		return err
	}

	// storage slot is already touched
	return s.aggregationStorage.Update(validator, agg)
}

// Renew transitions the validator's delegations to the next staking period.
// Called during staking period renewal process.
func (s *Service) Renew(validator thor.Address) (*globalstats.Renewal, uint64, error) {
	agg, err := s.GetAggregation(validator)
	if err != nil {
		return nil, 0, err
	}

	renew, err := agg.renew()
	if err != nil {
		return nil, 0, err
	}

	// storage slot is already touched
	if err = s.aggregationStorage.Update(validator, agg); err != nil {
		return nil, 0, err
	}

	return renew, agg.Locked.Weight, nil
}

// Exit moves all delegations to withdrawable state when validator exits.
// Called when a validator is removed from the active set.
func (s *Service) Exit(validator thor.Address) (*globalstats.Exit, error) {
	agg, err := s.GetAggregation(validator)
	if err != nil {
		return nil, err
	}

	exit := agg.exit()

	// storage slot is already touched
	if err = s.aggregationStorage.Update(validator, agg); err != nil {
		return nil, err
	}

	return exit, nil
}

// SignalExit marks locked delegations as exiting for the next period.
// Called when a validator signals intent to exit but hasn't exited yet.
func (s *Service) SignalExit(validator thor.Address, stake *stakes.WeightedStake) error {
	agg, err := s.GetAggregation(validator)
	if err != nil {
		return err
	}

	// Only move to exiting pools - don't subtract from locked yet
	// The subtraction happens during renewal
	if err = agg.Exiting.Add(stake); err != nil {
		return err
	}

	// storage slot is already touched
	return s.aggregationStorage.Update(validator, agg)
}
