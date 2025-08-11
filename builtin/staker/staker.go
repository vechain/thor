// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"

	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/aggregation"
	"github.com/vechain/thor/v2/builtin/staker/globalstats"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)



// TODO: Do these need to be set in params.sol, or some other dynamic way?
var (
	logger   = log.WithContext("pkg", "staker")
	MinStake = big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	MaxStake = big.NewInt(0).Mul(big.NewInt(600e6), big.NewInt(1e18))

	LowStakingPeriod    = solidity.NewConfigVariable("staker-low-staking-period", 360*24*7)     // 7 Days
	MediumStakingPeriod = solidity.NewConfigVariable("staker-medium-staking-period", 360*24*15) // 15 Days
	HighStakingPeriod   = solidity.NewConfigVariable("staker-high-staking-period", 360*24*30)   // 30 Days

	CooldownPeriod = solidity.NewConfigVariable("cooldown-period", 8640) // 8640 blocks, 1 day
	EpochLength    = solidity.NewConfigVariable("epoch-length", 180)     // 180 epochs
)

func SetLogger(l log.Logger) {
	logger = l
}

// Staker implements native methods of `Staker` contract.
type Staker struct {
	params *params.Params

	aggregationService *aggregation.Service
	globalStatsService *globalstats.Service
	validationService  *validation.Service
}

// New create a new instance.
func New(addr thor.Address, state *state.State, params *params.Params) *Staker {
	sctx := solidity.NewContext(addr, state)

	// debug overrides for testing
	LowStakingPeriod.Override(sctx)
	MediumStakingPeriod.Override(sctx)
	HighStakingPeriod.Override(sctx)
	CooldownPeriod.Override(sctx)
	EpochLength.Override(sctx)

	return &Staker{
		params: params,

		aggregationService: aggregation.New(sctx),
		globalStatsService: globalstats.New(sctx),
		validationService: validation.New(sctx),
	}
}

// IsPoSActive checks if the staker contract has become active, i.e. we have transitioned to PoS.
func (s *Staker) IsPoSActive() (bool, error) {
	return s.validationService.IsActive()
}

// LeaderGroup lists all registered candidates.
func (s *Staker) LeaderGroup() (map[thor.Address]*validation.Validation, error) {
	return s.validationService.LeaderGroup()
}

// LockedVET returns the amount of VET and weight locked by validations and delegations.
func (s *Staker) LockedVET() (*big.Int, *big.Int, error) {
	return s.globalStatsService.GetLockedVET()
}

// QueuedGroupSize returns the number of validations in the queue
func (s *Staker) QueuedGroupSize() (*big.Int, error) {
	return s.validationService.QueuedGroupSize()
}

// LeaderGroupSize returns the number of validations in the leader group
func (s *Staker) LeaderGroupSize() (*big.Int, error) {
	return s.validationService.LeaderGroupSize()
}
// Get returns a validation
func (s *Staker) Get(validator thor.Address) (*validation.Validation, error) {
	return s.validationService.GetValidation(validator)
}

// HasDelegations returns true if the validator has any delegations.
func (s *Staker) HasDelegations(
	node thor.Address,
) (bool, error) {
	agg, err := s.aggregationService.GetAggregation(node)
	if err != nil {
		return false, err
	}

	// Only return true if there is locked VET in the aggregation.
	return agg.LockedVET.Sign() == 1, nil
}

func (s *Staker) SetOnline(validator thor.Address, online bool) (bool, error) {
	logger.Debug("set node online", "validator", validator, "online", online)
	entry, err := s.validationService.GetValidation(validator)
	if err != nil {
		return false, err
	}
	hasChanged := entry.Online != online
	entry.Online = online
	if hasChanged {
		err = s.validationService.SetValidation(validator, entry, false)
	} else {
		err = nil
	}
	return hasChanged, err
}

// IncreaseDelegatorsReward Increases reward for validation's delegators.
func (s *Staker) IncreaseDelegatorsReward(node thor.Address, reward *big.Int) error {
	return s.validationService.IncreaseDelegatorsReward(node, reward)
}
