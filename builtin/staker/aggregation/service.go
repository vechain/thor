// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package aggregation

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/delta"
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
	d, err := s.aggregationStorage.Get(validator)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator aggregation")
	}

	// never return nil pointer aggregations
	// no need to check all properties it
	// should never happen a case where (e.g.) d.LockedVET == nil and d.WithdrawableVET != nil
	if d == nil || d.LockedVET == nil {
		d = newAggregation()
	}
	return d, nil
}

// AddPendingVET adds a new delegation to the validator's pending pool.
// Called when a delegator creates a new delegation.
func (s *Service) AddPendingVET(validator thor.Address, stake *stakes.WeightedStake) error {
	agg, err := s.GetAggregation(validator)
	if err != nil {
		return err
	}
	agg.PendingVET = big.NewInt(0).Add(agg.PendingVET, stake.VET())
	agg.PendingWeight = big.NewInt(0).Add(agg.PendingWeight, stake.Weight())

	return s.aggregationStorage.Set(validator, agg, false)
}

// SubPendingVet removes VET from the validator's pending pool.
// Called when a delegation is withdrawn before becoming active.
func (s *Service) SubPendingVet(validator thor.Address, stake *stakes.WeightedStake) error {
	agg, err := s.GetAggregation(validator)
	if err != nil {
		return err
	}
	agg.PendingVET = big.NewInt(0).Sub(agg.PendingVET, stake.VET())
	agg.PendingWeight = big.NewInt(0).Sub(agg.PendingWeight, stake.Weight())

	return s.aggregationStorage.Set(validator, agg, false)
}

// SubWithdrawableVET removes VET from the validator's withdrawable pool.
// Called when a delegator completes withdrawal of their delegation.
func (s *Service) SubWithdrawableVET(validator thor.Address, stake *big.Int) error {
	agg, err := s.GetAggregation(validator)
	if err != nil {
		return err
	}

	if agg.WithdrawableVET.Cmp(stake) < 0 {
		return errors.New("not enough withdraw VET")
	}

	agg.WithdrawableVET = big.NewInt(0).Sub(agg.WithdrawableVET, stake)

	return s.aggregationStorage.Set(validator, agg, false)
}

// Renew transitions the validator's delegations to the next staking period.
// Called during staking period renewal process.
func (s *Service) Renew(validator thor.Address) (*delta.Renewal, error) {
	agg, err := s.GetAggregation(validator)
	if err != nil {
		return nil, err
	}

	renew := agg.renew()

	if err = s.aggregationStorage.Set(validator, agg, false); err != nil {
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

	exit, err := agg.exit()
	if err != nil {
		return nil, err
	}

	if err = s.aggregationStorage.Set(validator, agg, false); err != nil {
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
	agg.ExitingVET = big.NewInt(0).Add(agg.ExitingVET, stake.VET())
	agg.ExitingWeight = big.NewInt(0).Add(agg.ExitingWeight, stake.Weight())

	return s.aggregationStorage.Set(validator, agg, false)
}
