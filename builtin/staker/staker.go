// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"errors"
	"fmt"
	"math/big"

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
	logger = log.WithContext("pkg", "staker")

	MinStakeVET = uint64(25e6)  // 25M VET
	MaxStakeVET = uint64(600e6) // 600M VET
	// exported for other packages to use
	MinStake = big.NewInt(0).Mul(new(big.Int).SetUint64(MinStakeVET), big.NewInt(1e18))
	MaxStake = big.NewInt(0).Mul(new(big.Int).SetUint64(MaxStakeVET), big.NewInt(1e18))
)

func SetLogger(l log.Logger) {
	logger = l
}

// Staker implements native methods of `Staker` contract.
type Staker struct {
	params *params.Params
	state  *state.State

	aggregationService *aggregation.Service
	globalStatsService *globalstats.Service
	validationService  *validation.Service
	delegationService  *delegation.Service
}

// New create a new instance.
func New(addr thor.Address, state *state.State, params *params.Params, charger *gascharger.Charger) *Staker {
	sctx := solidity.NewContext(addr, state, charger)

	return &Staker{
		params: params,
		state:  state,

		aggregationService: aggregation.New(sctx),
		globalStatsService: globalstats.New(sctx),
		delegationService:  delegation.New(sctx),
		validationService: validation.New(
			sctx,
			MinStakeVET,
			MaxStakeVET,
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
func (s *Staker) LeaderGroup() ([]validation.Leader, error) {
	return s.validationService.LeaderGroup()
}

// LockedStake returns the amount of VET and weight locked by validations and delegations.
func (s *Staker) LockedStake() (uint64, uint64, error) {
	return s.globalStatsService.GetLockedStake()
}

// QueuedStake returns the amount of VET queued by validations and delegations.
func (s *Staker) QueuedStake() (uint64, error) {
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
func (s *Staker) QueuedGroupSize() (uint64, error) {
	return s.validationService.QueuedGroupSize()
}

// LeaderGroupSize returns the number of validations in the leader group
func (s *Staker) LeaderGroupSize() (uint64, error) {
	return s.validationService.LeaderGroupSize()
}

// GetValidation returns a validation
func (s *Staker) GetValidation(validator thor.Address) (*validation.Validation, error) {
	return s.validationService.GetValidation(validator)
}

// GetWithdrawable returns the withdrawable stake of a validator.
func (s *Staker) GetWithdrawable(validator thor.Address, block uint32) (uint64, error) {
	val, err := s.validationService.GetExistingValidation(validator)
	if err != nil {
		return 0, err
	}

	return val.CalculateWithdrawableVET(block), err
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
		return del, nil, nil
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
	return agg.LockedVET > 0, nil
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
	return val.Totals(agg)
}

// Next returns the next validator in a linked list.
// If the provided address is not in a list, it will return empty bytes.
func (s *Staker) Next(prev thor.Address) (thor.Address, error) {
	// First check leader group
	next, err := s.validationService.NextEntry(prev)
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
	endorser thor.Address,
	period uint32,
	stake uint64,
) error {
	logger.Debug("adding validator", "validator", validator,
		"endorser", endorser,
		"period", period,
		"stake", stake,
	)

	if stake < MinStakeVET || stake > MaxStakeVET {
		return NewReverts("stake is out of range")
	}

	val, err := s.validationService.GetValidation(validator)
	if err != nil {
		return err
	}
	if !val.IsEmpty() {
		return NewReverts("validator already exists")
	}

	if period != thor.LowStakingPeriod() && period != thor.MediumStakingPeriod() && period != thor.HighStakingPeriod() {
		return NewReverts("period is out of boundaries")
	}

	// create a new validation
	if err := s.validationService.Add(validator, endorser, period, stake); err != nil {
		logger.Info("add validator failed", "validator", validator, "error", err)
		return err
	}

	// update global totals
	if err := s.globalStatsService.AddQueued(stake); err != nil {
		return err
	}

	logger.Info("added validator", "validator", validator)
	return nil
}

func (s *Staker) SignalExit(validator thor.Address, endorser thor.Address) error {
	logger.Debug("signal exit", "endorser", endorser, "validator", validator)

	val, err := s.validationService.GetValidation(validator)
	if err != nil {
		return err
	}
	if val.IsEmpty() {
		return NewReverts("validation does not exist")
	}
	if val.Endorser != endorser {
		return NewReverts("endorser required")
	}
	if val.Status != validation.StatusActive {
		return NewReverts("can't signal exit while not active")
	}

	if val.ExitBlock != nil {
		return NewReverts(fmt.Sprintf("exit block already set to %d", *val.ExitBlock))
	}

	if err := s.validationService.SignalExit(validator, val); err != nil {
		if errors.Is(err, validation.ErrMaxTryReached) {
			return NewReverts(validation.ErrMaxTryReached.Error())
		}
		logger.Info("signal exit failed", "validator", validator, "error", err)
		return err
	}

	return nil
}

// IncreaseStake increases the stake of a queued or active validator
// if a validator is active, the QueuedVET is increase, but the weight stays the same
// the weight will be recalculated at the end of the staking period, by the housekeep function
func (s *Staker) IncreaseStake(validator thor.Address, endorser thor.Address, amount uint64) error {
	logger.Debug("increasing stake", "endorser", endorser, "validator", validator, "amount", amount)

	val, err := s.validationService.GetValidation(validator)
	if err != nil {
		return err
	}
	if val.IsEmpty() {
		return NewReverts("validation does not exist")
	}
	if val.Endorser != endorser {
		return NewReverts("endorser required")
	}
	if val.Status == validation.StatusExit {
		return NewReverts("validator exited")
	}
	if val.Status == validation.StatusActive && val.ExitBlock != nil {
		return NewReverts("validator has signaled exit, cannot increase stake")
	}

	// validate that new TVL is <= Max stake
	if err := s.validateStakeIncrease(validator, val, amount); err != nil {
		return err
	}

	if err := s.validationService.IncreaseStake(validator, val, amount); err != nil {
		logger.Info("increase stake failed", "validator", validator, "error", err)
		return err
	}

	// update global queued, use the initial multiplier
	if err := s.globalStatsService.AddQueued(amount); err != nil {
		return err
	}

	logger.Info("increased stake", "validator", validator)
	return nil
}

func (s *Staker) DecreaseStake(validator thor.Address, endorser thor.Address, amount uint64) error {
	logger.Debug("decreasing stake", "endorser", endorser, "validator", validator, "amount", amount)

	val, err := s.GetValidation(validator)
	if err != nil {
		return err
	}
	if val.IsEmpty() {
		return NewReverts("validation does not exist")
	}
	if val.Endorser != endorser {
		return NewReverts("endorser required")
	}
	if val.Status == validation.StatusExit {
		return NewReverts("validator exited")
	}
	if val.Status == validation.StatusActive && val.ExitBlock != nil {
		return NewReverts("validator has signaled exit, cannot decrease stake")
	}

	if val.Status == validation.StatusActive {
		// We don't consider any increases, i.e., entry.QueuedVET. We only consider locked and current decreases.
		// The reason is that validator can instantly withdraw QueuedVET at any time.
		// We need to make sure the locked VET minus the sum of the current decreases is still above the minimum stake.
		pendingAndDecrease := val.PendingUnlockVET + amount
		if pendingAndDecrease > val.LockedVET || val.LockedVET-pendingAndDecrease < MinStakeVET {
			return NewReverts("next period stake is lower than minimum stake")
		}
	}

	if val.Status == validation.StatusQueued {
		// All the validator's stake exists within QueuedVET, so we need to make sure it maintains a minimum of MinStake.
		if val.QueuedVET-amount < MinStakeVET {
			return NewReverts("next period stake is lower than minimum stake")
		}
	}

	if err = s.validationService.DecreaseStake(validator, val, amount); err != nil {
		logger.Info("decrease stake failed", "validator", validator, "error", err)
		return err
	}

	if val.Status == validation.StatusQueued {
		// update global totals, use the initial multiplier
		err = s.globalStatsService.RemoveQueued(amount)
		if err != nil {
			return err
		}
	}

	logger.Info("decreased stake", "validator", validator)
	return nil
}

// WithdrawStake allows expired validations to withdraw their stake.
func (s *Staker) WithdrawStake(validator thor.Address, endorser thor.Address, currentBlock uint32) (uint64, error) {
	logger.Debug("withdrawing stake", "endorser", endorser, "validator", validator)
	val, err := s.GetValidation(validator)
	if err != nil {
		return 0, err
	}
	if val.IsEmpty() {
		return 0, NewReverts("validation does not exist")
	}
	if val.Endorser != endorser {
		return 0, NewReverts("endorser required")
	}

	stake, queued, err := s.validationService.WithdrawStake(validator, val, currentBlock)
	if err != nil {
		logger.Info("withdraw failed", "validator", validator, "error", err)
		return 0, err
	}

	// remove validator QueuedVET if the validator is still queued or had a pending increase
	if queued > 0 {
		err = s.globalStatsService.RemoveQueued(queued)
		if err != nil {
			return 0, err
		}
	}

	logger.Info("withdrew validator staker", "validator", validator)
	return stake, nil
}

func (s *Staker) SetOnline(validator thor.Address, blockNum uint32, online bool) error {
	logger.Debug("set node online", "validator", validator, "online", online)
	return s.validationService.UpdateOfflineBlock(validator, blockNum, online)
}

func (s *Staker) SetBeneficiary(validator, endorser, beneficiary thor.Address) error {
	logger.Debug("set beneficiary", "validator", validator, "beneficiary", beneficiary)

	val, err := s.GetValidation(validator)
	if err != nil {
		return err
	}
	if val.IsEmpty() {
		return NewReverts("validation does not exist")
	}
	if val.Endorser != endorser {
		return NewReverts("endorser required")
	}
	if val.Status == validation.StatusExit || val.ExitBlock != nil {
		return NewReverts("validator has exited or signaled exit, cannot set beneficiary")
	}

	if err := s.validationService.SetBeneficiary(validator, val, beneficiary); err != nil {
		logger.Info("set beneficiary failed", "validator", validator, "error", err)
		return err
	}
	return nil
}

// AddDelegation adds a new delegation.
func (s *Staker) AddDelegation(
	validator thor.Address,
	stake uint64,
	multiplier uint8,
) (*big.Int, error) {
	logger.Debug("adding delegation", "validator", validator, "stake", stake, "multiplier", multiplier)

	if multiplier == 0 {
		return nil, NewReverts("multiplier cannot be 0")
	}
	// ensure validation is ok to receive a new delegation
	val, err := s.validationService.GetValidation(validator)
	if err != nil {
		return nil, err
	}
	if val.IsEmpty() {
		return nil, NewReverts("validation does not exist")
	}

	if val.Status != validation.StatusQueued && val.Status != validation.StatusActive {
		return nil, NewReverts("validation is not queued or active")
	}

	// validate that new TVL is <= Max stake
	if err = s.validateStakeIncrease(validator, val, stake); err != nil {
		return nil, err
	}

	// add delegation on the next iteration - val.CurrentIteration() + 1,
	delegationID, err := s.delegationService.Add(validator, val.CurrentIteration()+1, stake, multiplier)
	if err != nil {
		logger.Info("failed to add delegation", "validator", validator, "error", err)
		return nil, err
	}
	weightedStake := stakes.NewWeightedStakeWithMultiplier(stake, multiplier)

	if err = s.aggregationService.AddPendingVET(validator, weightedStake); err != nil {
		return nil, err
	}

	// update global figures
	if err = s.globalStatsService.AddQueued(stake); err != nil {
		return nil, err
	}

	logger.Info("added delegation", "validator", validator, "delegationID", delegationID)
	return delegationID, nil
}

// SignalDelegationExit updates the auto-renewal status of a delegation.
func (s *Staker) SignalDelegationExit(delegationID *big.Int) error {
	logger.Debug("signal delegation exit", "delegationID", delegationID)

	del, err := s.delegationService.GetDelegation(delegationID)
	if err != nil {
		return err
	}
	if del.IsEmpty() {
		return NewReverts("delegation is empty")
	}
	if del.LastIteration != nil {
		return NewReverts("delegation is already signaled exit")
	}
	if del.Stake == 0 {
		return NewReverts("delegation has already been withdrawn")
	}

	val, err := s.validationService.GetValidation(del.Validation)
	if err != nil {
		return err
	}

	// ensure delegation can be signaled ( delegation has started and has not ended )
	if !del.Started(val) {
		return NewReverts("delegation has not started yet, funds can be withdrawn")
	}
	if del.Ended(val) {
		return NewReverts("delegation has ended, funds can be withdrawn")
	}

	if err = s.delegationService.SignalExit(del, delegationID, val.CurrentIteration()); err != nil {
		logger.Info("signal delegation exit failed", "delegationID", delegationID, "error", err)
		return err
	}

	err = s.aggregationService.SignalExit(del.Validation, del.WeightedStake())
	if err != nil {
		return err
	}

	logger.Info("signal delegation exit", "delegationID", delegationID)
	return nil
}

// WithdrawDelegation allows expired and queued delegations to withdraw their stake.
func (s *Staker) WithdrawDelegation(
	delegationID *big.Int,
) (uint64, error) {
	logger.Debug("withdrawing delegation", "delegationID", delegationID)

	del, err := s.delegationService.GetDelegation(delegationID)
	if err != nil {
		return 0, err
	}

	val, err := s.validationService.GetValidation(del.Validation)
	if err != nil {
		return 0, err
	}

	// ensure the delegation is either queued or finished
	started := del.Started(val)
	finished := del.Ended(val)
	if started && !finished {
		return 0, NewReverts("delegation is not eligible for withdraw")
	}

	// withdraw delegation
	withdrawableStake, err := s.delegationService.Withdraw(del, delegationID, val)
	if err != nil {
		logger.Info("failed to withdraw", "delegationID", delegationID, "error", err)
		return 0, err
	}

	// start and finish values are sanitized: !started and finished is impossible
	// delegation is still queued
	if !started {
		weightedStake := stakes.NewWeightedStakeWithMultiplier(withdrawableStake, del.Multiplier)
		if err = s.aggregationService.SubPendingVet(del.Validation, weightedStake); err != nil {
			return 0, err
		}

		if err = s.globalStatsService.RemoveQueued(withdrawableStake); err != nil {
			return 0, err
		}
	}

	logger.Info("withdrew delegation", "delegationID", delegationID, "stake", withdrawableStake)
	return withdrawableStake, nil
}

// IncreaseDelegatorsReward Increases reward for validation's delegators.
func (s *Staker) IncreaseDelegatorsReward(node thor.Address, reward *big.Int) error {
	return s.validationService.IncreaseDelegatorsReward(node, reward)
}

func (s *Staker) validateStakeIncrease(validator thor.Address, validation *validation.Validation, amount uint64) error {
	agg, err := s.aggregationService.GetAggregation(validator)
	if err != nil {
		return err
	}

	// accumulated TVL should cannot be more than MaxStake
	aggNextPeriodTVL, err := agg.NextPeriodTVL()
	if err != nil {
		return err
	}
	valNextPeriodTVL, err := validation.NextPeriodTVL()
	if err != nil {
		return err
	}
	if valNextPeriodTVL+aggNextPeriodTVL+amount > MaxStakeVET {
		return NewReverts("stake is out of range")
	}

	return nil
}

// GetValidationsNum returns the number of validators in the leader group and number of queued validators.
func (s *Staker) GetValidationsNum() (uint64, uint64, error) {
	leaderGroupSize, err := s.LeaderGroupSize()
	if err != nil {
		return 0, 0, err
	}
	queuedGroupSize, err := s.QueuedGroupSize()
	if err != nil {
		return 0, 0, err
	}
	return leaderGroupSize, queuedGroupSize, nil
}
