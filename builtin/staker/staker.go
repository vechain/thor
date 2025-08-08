// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/aggregation"
	"github.com/vechain/thor/v2/builtin/staker/delegation"
	"github.com/vechain/thor/v2/builtin/staker/globalstats"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
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
	delegationService  *delegation.Service
}

// New create a new instance.
func New(addr thor.Address, state *state.State, params *params.Params, charger *gascharger.Charger) *Staker {
	sctx := solidity.NewContext(addr, state, charger)

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
		delegationService:  delegation.New(sctx),
		validationService: validation.New(
			sctx,
			CooldownPeriod.Get(),
			EpochLength.Get(),
			LowStakingPeriod.Get(),
			MediumStakingPeriod.Get(),
			HighStakingPeriod.Get(),
			MinStake,
			MaxStake,
		),
	}
}

//
// Getters - no state change
//

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

// QueuedStake returns the amount of VET and weight queued by validations and delegations.
func (s *Staker) QueuedStake() (*big.Int, *big.Int, error) {
	return s.globalStatsService.GetQueuedStake()
}

// FirstActive returns validator address of first entry.
func (s *Staker) FirstActive() (*thor.Address, error) {
	return s.validationService.FirstActive()
}

// FirstQueued returns validator address of first entry.
func (s *Staker) FirstQueued() (*thor.Address, error) {
	return s.validationService.FirstQueued()
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

// GetWithdrawable returns the withdrawable stake of a validator.
func (s *Staker) GetWithdrawable(validator thor.Address, block uint32) (*big.Int, error) {
	val, err := s.validationService.GetExistingValidation(validator)
	if err != nil {
		return nil, err
	}

	return val.CalculateWithdrawableVET(block, CooldownPeriod.Get()), err
}

// GetDelegation returns the delegation.
func (s *Staker) GetDelegation(
	delegationID *big.Int,
) (*delegation.Delegation, *validation.Validation, error) {
	del, err := s.delegationService.GetDelegation(delegationID)
	if err != nil {
		return nil, nil, err
	}
	if del.IsEmpty() {
		return nil, nil, nil
	}
	val, err := s.validationService.GetValidation(del.Validation)
	if err != nil {
		return nil, nil, err
	}
	return del, val, nil
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

// GetDelegatorRewards returns reward amount for validator and staking period.
func (s *Staker) GetDelegatorRewards(validator thor.Address, stakingPeriod uint32) (*big.Int, error) {
	return s.validationService.GetDelegatorRewards(validator, stakingPeriod)
}

// GetCompletedPeriods returns number of completed staking periods for validation.
func (s *Staker) GetCompletedPeriods(validator thor.Address) (uint32, error) {
	return s.validationService.GetCompletedPeriods(validator)
}

// GetValidationTotals returns the total stake, total weight, total delegators stake and total delegators weight.
func (s *Staker) GetValidationTotals(validator thor.Address) (*validation.Totals, error) {
	val, err := s.validationService.GetValidation(validator)
	if err != nil {
		return nil, err
	}
	agg, err := s.aggregationService.GetAggregation(validator)
	if err != nil {
		return nil, err
	}
	return val.Totals(agg), nil
}

// Next returns the next validator in a linked list.
// If the provided address is not in a list, it will return empty bytes.
func (s *Staker) Next(prev thor.Address) (thor.Address, error) {
	// First check leader group
	next, err := s.validationService.LeaderGroupNext(prev)
	if err != nil {
		return thor.Address{}, err
	}
	if !next.IsZero() {
		return next, nil
	}

	// Then check validator queue
	next, err = s.validationService.ValidatorQueueNext(prev)
	if err != nil {
		return thor.Address{}, err
	}
	return next, nil
}

//
// Setters - state change
//

// AddValidation queues a new validator.
func (s *Staker) AddValidation(
	validator thor.Address,
	endorsor thor.Address,
	period uint32,
	stake *big.Int,
) error {
	logger.Debug("adding validator", "validator", validator,
		"endorsor", endorsor,
		"period", period,
		"stake", new(big.Int).Div(stake, big.NewInt(1e18)),
	)

	// create a new validation
	if err := s.validationService.Add(validator, endorsor, period, stake); err != nil {
		logger.Info("add validator failed", "validator", validator, "error", err)
		return err
	}

	// update global totals
	err := s.globalStatsService.AddQueued(validation.WeightedStake(stake))
	if err != nil {
		return err
	}

	logger.Info("added validator", "validator", validator)
	return nil
}

func (s *Staker) SignalExit(validator thor.Address, endorsor thor.Address) error {
	logger.Debug("signal exit", "endorsor", endorsor, "validator", validator)

	if err := s.validationService.SignalExit(validator, endorsor); err != nil {
		logger.Info("signal exit failed", "validator", validator, "error", err)
		return err
	}

	return nil
}

// IncreaseStake increases the stake of a queued or active validator
// if a validator is active, the QueuedVET is increase, but the weight stays the same
// the weight will be recalculated at the end of the staking period, by the housekeep function
func (s *Staker) IncreaseStake(validator thor.Address, endorsor thor.Address, amount *big.Int) error {
	logger.Debug("increasing stake", "endorsor", endorsor, "validator", validator, "amount", new(big.Int).Div(amount, big.NewInt(1e18)))

	if err := s.validationService.IncreaseStake(validator, endorsor, amount); err != nil {
		logger.Info("increase stake failed", "validator", validator, "error", err)
		return err
	}

	// validate that new TVL is <= Max stake
	if err := s.validateNextPeriodTVL(validator); err != nil {
		return err
	}

	// update global totals
	if err := s.globalStatsService.AddQueued(validation.WeightedStake(amount)); err != nil {
		return err
	}

	logger.Info("increased stake", "validator", validator)
	return nil
}

func (s *Staker) DecreaseStake(validator thor.Address, endorsor thor.Address, amount *big.Int) error {
	logger.Debug("decreasing stake", "endorsor", endorsor, "validator", validator, "amount", new(big.Int).Div(amount, big.NewInt(1e18)))

	queued, err := s.validationService.DecreaseStake(validator, endorsor, amount)
	if err != nil {
		logger.Info("decrease stake failed", "validator", validator, "error", err)
		return err
	}

	if queued {
		err = s.globalStatsService.RemoveQueued(validation.WeightedStake(amount))
		if err != nil {
			return err
		}
	}

	logger.Info("decreased stake", "validator", validator)
	return nil
}

// WithdrawStake allows expired validations to withdraw their stake.
func (s *Staker) WithdrawStake(validator thor.Address, endorsor thor.Address, currentBlock uint32) (*big.Int, error) {
	logger.Debug("withdrawing stake", "endorsor", endorsor, "validator", validator)

	// remove validator QueuedVET if the validator is still queued
	val, err := s.validationService.GetValidation(validator)
	if err != nil {
		return nil, err
	}
	if val.Status == validation.StatusQueued {
		err = s.globalStatsService.RemoveQueued(validation.WeightedStake(val.QueuedVET))
		if err != nil {
			return nil, err
		}
	}

	stake, err := s.validationService.WithdrawStake(val, validator, endorsor, currentBlock)
	if err != nil {
		logger.Info("withdraw failed", "validator", validator, "error", err)
		return nil, err
	}

	logger.Info("withdrew validator staker", "validator", validator)
	return stake, nil
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

// AddDelegation adds a new delegation.
func (s *Staker) AddDelegation(
	validator thor.Address,
	stake *big.Int,
	multiplier uint8,
) (*big.Int, error) {
	logger.Debug("adding delegation", "validator", validator, "stake", new(big.Int).Div(stake, big.NewInt(1e18)), "multiplier", multiplier)

	// ensure validation is ok to receive a new delegation
	val, err := s.validationService.GetExistingValidation(validator)
	if err != nil {
		return nil, err
	}

	if val.Status != validation.StatusQueued && val.Status != validation.StatusActive {
		return nil, errors.New("validation is not queued or active")
	}

	// add delegation on the next iteration - val.CurrentIteration() + 1,
	delegationID, err := s.delegationService.Add(validator, val.CurrentIteration()+1, stake, multiplier)
	if err != nil {
		logger.Info("failed to add delegation", "validator", validator, "error", err)
		return nil, err
	}
	weightedStake := stakes.NewWeightedStake(stake, multiplier)

	if err = s.aggregationService.AddPendingVET(validator, weightedStake); err != nil {
		return nil, err
	}

	// validate that new TVL is <= Max stake
	if err = s.validateNextPeriodTVL(validator); err != nil {
		return nil, err
	}

	// update global figures
	if err = s.globalStatsService.AddQueued(weightedStake); err != nil {
		return nil, err
	}

	logger.Info("added delegation", "validator", validator, "delegationID", delegationID)
	return delegationID, nil
}

// SignalDelegationExit updates the auto-renewal status of a delegation.
func (s *Staker) SignalDelegationExit(delegationID *big.Int) error {
	logger.Debug("updating autorenew", "delegationID", delegationID)

	del, err := s.delegationService.GetDelegation(delegationID)
	if err != nil {
		return err
	}

	if del.IsEmpty() {
		return errors.New("delegation is empty")
	}

	val, err := s.validationService.GetValidation(del.Validation)
	if err != nil {
		return err
	}

	// ensure delegation can be signaled ( delegation has started and has not ended )
	if !del.Started(val) {
		return errors.New("delegation has not started yet, funds can be withdrawn")
	}
	if del.Ended(val) {
		return errors.New("delegation has ended, funds can be withdrawn")
	}

	if err = s.delegationService.SignalExit(del, delegationID, val.CurrentIteration()); err != nil {
		logger.Info("update autorenew failed", "delegationID", delegationID, "error", err)
		return err
	}

	err = s.aggregationService.SignalExit(del.Validation, del.WeightedStake())
	if err != nil {
		return err
	}

	logger.Info("updated autorenew", "delegationID", delegationID)
	return nil
}

// WithdrawDelegation allows expired and queued delegations to withdraw their stake.
func (s *Staker) WithdrawDelegation(
	delegationID *big.Int,
) (*big.Int, error) {
	logger.Debug("withdrawing delegation", "delegationID", delegationID)

	del, err := s.delegationService.GetDelegation(delegationID)
	if err != nil {
		return nil, err
	}

	val, err := s.validationService.GetValidation(del.Validation)
	if err != nil {
		return nil, err
	}

	// ensure the delegation is either queued or finished
	started := del.Started(val)
	finished := del.Ended(val)
	if started && !finished {
		return nil, errors.New("delegation is not eligible for withdraw")
	}

	// withdraw delegation
	withdrawableStake, err := s.delegationService.Withdraw(del, delegationID)
	if err != nil {
		logger.Info("failed to withdraw", "delegationID", delegationID, "error", err)
		return nil, err
	}

	// start and finish values are sanitized: !started and finished is impossible
	// delegation is still queued
	if !started {
		weightedStake := stakes.NewWeightedStake(withdrawableStake, del.Multiplier)

		if err = s.aggregationService.SubPendingVet(del.Validation, weightedStake); err != nil {
			return nil, err
		}

		if err = s.globalStatsService.RemoveQueued(weightedStake); err != nil {
			return nil, err
		}
	}

	logger.Info("withdrew delegation", "delegationID", delegationID, "stake", new(big.Int).Div(withdrawableStake, big.NewInt(1e18)))
	return withdrawableStake, nil
}

// IncreaseDelegatorsReward Increases reward for validation's delegators.
func (s *Staker) IncreaseDelegatorsReward(node thor.Address, reward *big.Int) error {
	return s.validationService.IncreaseDelegatorsReward(node, reward)
}

func (s *Staker) validateNextPeriodTVL(validator thor.Address) error {
	val, err := s.validationService.GetValidation(validator)
	if err != nil {
		return err
	}

	agg, err := s.aggregationService.GetAggregation(validator)
	if err != nil {
		return err
	}

	// accumulated TVL should cannot be more than MaxStake
	if big.NewInt(0).Add(val.NextPeriodTVL(), agg.NextPeriodTVL()).Cmp(MaxStake) > 0 {
		return errors.New("stake is out of range")
	}

	return nil
}

// GetValidatorsNum returns the number of validators in the leader group and number of queued validators.
func (s *Staker) GetValidatorsNum() (*big.Int, *big.Int, error) {
	leaderGroupSize, err := s.LeaderGroupSize()
	if err != nil {
		return nil, nil, err
	}
	queuedGroupSize, err := s.QueuedGroupSize()
	if err != nil {
		return leaderGroupSize, nil, err
	}
	return leaderGroupSize, queuedGroupSize, nil
}
