// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// NOTE: As a general rule to the staker package:
// All complex structs should be passed by pointer.
// All non-complex structs should be passed by value.
// It is considered thor.Address and thor.Bytes32 non-complex structs.

package staker

import (
	"errors"
	"fmt"
	"math/big"
	"math/bits"

	"github.com/ethereum/go-ethereum/common/math"

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

const (
	MinStakeVET = uint64(25e6)  // 25M VET
	MaxStakeVET = uint64(600e6) // 600M VET
)

var (
	logger = log.WithContext("pkg", "staker")

	// exported for other packages to use
	MinStake = big.NewInt(0).Mul(new(big.Int).SetUint64(MinStakeVET), big.NewInt(1e18))
	MaxStake = big.NewInt(0).Mul(new(big.Int).SetUint64(MaxStakeVET), big.NewInt(1e18))

	exitMaxTry = 20 // revert transaction if after these attempts an exit block is not found
)

func SetLogger(l log.Logger) {
	logger = l
}

// Staker implements native methods of `Staker` contract.
type Staker struct {
	params  *params.Params
	state   *state.State
	address thor.Address

	aggregationService *aggregation.Service
	globalStatsService *globalstats.Service
	validationService  *validation.Service
	delegationService  *delegation.Service
}

// New create a new instance.
func New(addr thor.Address, state *state.State, params *params.Params, charger *gascharger.Charger) *Staker {
	sctx := solidity.NewContext(addr, state, charger)

	return &Staker{
		params:  params,
		state:   state,
		address: addr,

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
func (s *Staker) FirstActive() (thor.Address, error) {
	return s.validationService.FirstActive()
}

// FirstQueued returns validator address of first entry.
func (s *Staker) FirstQueued() (thor.Address, error) {
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
	val, err := s.getValidationOrRevert(validator)
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
	if del == nil {
		return nil, nil, nil
	}
	// any valid delegation must have a valid validation
	val, err := s.validationService.GetExistingValidation(del.Validation)
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
	return agg.Locked.VET > 0, nil
}

// GetDelegatorRewards returns reward amount for validator and staking period.
func (s *Staker) GetDelegatorRewards(validator thor.Address, stakingPeriod uint32) (*big.Int, error) {
	return s.validationService.GetDelegatorRewards(validator, stakingPeriod)
}

// GetValidationTotals returns the total stake, total weight, total delegators stake and total delegators weight.
func (s *Staker) GetValidationTotals(validator thor.Address) (*validation.Totals, error) {
	val, err := s.getValidationOrRevert(validator)
	if err != nil {
		return nil, err
	}

	// GetAggregation ensures new aggregation is created if not found
	agg, err := s.aggregationService.GetAggregation(validator)
	if err != nil {
		return nil, err
	}
	return val.Totals(agg)
}

// Next returns the next validator in a linked list.
// If the provided address is not in a list, it will return empty bytes.
func (s *Staker) Next(prev thor.Address) (thor.Address, error) {
	return s.validationService.NextEntry(prev)
}

func (s *Staker) Address() thor.Address {
	return s.address
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
	if stake < MinStakeVET {
		return NewReverts("stake is below minimum")
	}
	if stake > MaxStakeVET {
		return NewReverts("stake is above maximum")
	}

	if validator.IsZero() {
		return NewReverts("validator cannot be zero")
	}

	val, err := s.validationService.GetValidation(validator)
	if err != nil {
		return err
	}
	if val != nil {
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

	if err = s.ContractBalanceCheck(0); err != nil {
		return err
	}

	logger.Info("added validator", "validator", validator)
	return nil
}

func (s *Staker) SignalExit(validator thor.Address, endorser thor.Address, currentBlock uint32) error {
	logger.Debug("signal exit", "endorser", endorser, "validator", validator)

	val, err := s.getValidationOrRevert(validator)
	if err != nil {
		return err
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

	current, err := val.CurrentIteration(currentBlock)
	if err != nil {
		return err
	}
	minBlock := val.StartBlock + val.Period*current
	if err := s.validationService.SignalExit(validator, currentBlock, minBlock, exitMaxTry); err != nil {
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

	val, err := s.getValidationOrRevert(validator)
	if err != nil {
		return err
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

	if val.Status == validation.StatusActive {
		if err = s.validationService.AddToRenewalList(validator); err != nil {
			return err
		}
	}

	// update global queued, use the initial multiplier
	if err := s.globalStatsService.AddQueued(amount); err != nil {
		return err
	}

	if err = s.ContractBalanceCheck(0); err != nil {
		return err
	}

	logger.Info("increased stake", "validator", validator)
	return nil
}

func (s *Staker) DecreaseStake(validator thor.Address, endorser thor.Address, amount uint64) error {
	logger.Debug("decreasing stake", "endorser", endorser, "validator", validator, "amount", amount)
	if amount > MaxStakeVET-MinStakeVET {
		return NewReverts("decrease amount is too large")
	}

	val, err := s.getValidationOrRevert(validator)
	if err != nil {
		return err
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

	var nextPeriodVET uint64
	if val.Status == validation.StatusActive {
		// We don't consider any increases, i.e., entry.QueuedVET. We only consider locked and current decreases.
		// The reason is that validator can instantly withdraw QueuedVET at any time.
		// We need to make sure the locked VET minus the sum of the current decreases is still above the minimum stake.
		nextPeriodVET = val.LockedVET - val.PendingUnlockVET
	}
	if val.Status == validation.StatusQueued {
		nextPeriodVET = val.QueuedVET
	}
	if amount > nextPeriodVET {
		return NewReverts("not enough locked stake")
	}
	if nextPeriodVET-amount < MinStakeVET {
		return NewReverts("next period stake is lower than minimum stake")
	}

	if err = s.validationService.DecreaseStake(validator, val, amount); err != nil {
		logger.Info("decrease stake failed", "validator", validator, "error", err)
		return err
	}

	if val.Status == validation.StatusActive {
		if err = s.validationService.AddToRenewalList(validator); err != nil {
			return err
		}
	}

	if val.Status == validation.StatusQueued {
		// update global queued
		if err = s.globalStatsService.RemoveQueued(amount); err != nil {
			return err
		}

		// update global withdrawable
		if err = s.globalStatsService.AddWithdrawable(amount); err != nil {
			return err
		}
	}

	if err = s.ContractBalanceCheck(0); err != nil {
		return err
	}

	logger.Info("decreased stake", "validator", validator)

	return nil
}

// WithdrawStake allows expired validations to withdraw their stake.
func (s *Staker) WithdrawStake(validator thor.Address, endorser thor.Address, currentBlock uint32) (uint64, error) {
	logger.Debug("withdrawing stake", "endorser", endorser, "validator", validator)
	val, err := s.getValidationOrRevert(validator)
	if err != nil {
		return 0, err
	}

	if val.Endorser != endorser {
		return 0, NewReverts("endorser required")
	}

	withdrawableVET, queuedVET, cooldownVET, err := s.validationService.WithdrawStake(validator, val, currentBlock)
	if err != nil {
		logger.Info("withdraw failed", "validator", validator, "error", err)
		return 0, err
	}

	// update global stats
	if withdrawableVET > 0 {
		if err := s.globalStatsService.RemoveWithdrawable(withdrawableVET); err != nil {
			return 0, err
		}
	}
	if queuedVET > 0 {
		if err = s.globalStatsService.RemoveQueued(queuedVET); err != nil {
			return 0, err
		}
	}
	if cooldownVET > 0 {
		if err = s.globalStatsService.RemoveCooldown(cooldownVET); err != nil {
			return 0, err
		}
	}
	total, overflow := math.SafeAdd(withdrawableVET, queuedVET)
	if overflow {
		return 0, errors.New("withdrawableVET/ queuedVET overflow")
	}
	total, overflow = math.SafeAdd(total, cooldownVET)
	if overflow {
		return 0, errors.New("cooldownVET caused overflow")
	}

	if err = s.ContractBalanceCheck(total); err != nil {
		return 0, err
	}

	logger.Info("withdrew stake", "validator", validator, "amount", total)

	return total, nil
}

func (s *Staker) SetOnline(validator thor.Address, blockNum uint32, online bool) error {
	logger.Debug("set node online", "validator", validator, "online", online)
	return s.validationService.UpdateOfflineBlock(validator, blockNum, online)
}

func (s *Staker) SetBeneficiary(validator, endorser, beneficiary thor.Address) error {
	logger.Debug("set beneficiary", "validator", validator, "beneficiary", beneficiary)

	val, err := s.getValidationOrRevert(validator)
	if err != nil {
		return err
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
	currentBlock uint32,
) (*big.Int, error) {
	logger.Debug("adding delegation", "validator", validator, "stake", stake, "multiplier", multiplier)

	if stake <= 0 {
		return nil, NewReverts("stake must be greater than 0")
	}

	if multiplier == 0 {
		return nil, NewReverts("multiplier cannot be 0")
	}
	// ensure validation is ok to receive a new delegation
	val, err := s.getValidationOrRevert(validator)
	if err != nil {
		return nil, err
	}

	if val.Status != validation.StatusQueued && val.Status != validation.StatusActive {
		return nil, NewReverts("validation is not queued or active")
	}

	// delegations cannot be added to a validator that has signaled to exit
	if val.ExitBlock != nil {
		return nil, NewReverts("cannot add delegation to exiting validator")
	}

	// validate that new TVL is <= Max stake
	if err = s.validateStakeIncrease(validator, val, stake); err != nil {
		return nil, err
	}

	// add delegation on the next iteration - val.CurrentIteration() + 1,
	current, err := val.CurrentIteration(currentBlock)
	if err != nil {
		return nil, err
	}
	delegationID, err := s.delegationService.Add(validator, current+1, stake, multiplier)
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

	if val.Status == validation.StatusActive {
		if err = s.validationService.AddToRenewalList(validator); err != nil {
			return nil, err
		}
	}

	if err = s.ContractBalanceCheck(0); err != nil {
		return nil, err
	}

	logger.Info("added delegation", "validator", validator, "delegationID", delegationID)
	return delegationID, nil
}

// SignalDelegationExit updates the auto-renewal status of a delegation.
func (s *Staker) SignalDelegationExit(delegationID *big.Int, currentBlock uint32) error {
	logger.Debug("signal delegation exit", "delegationID", delegationID)

	del, err := s.delegationService.GetDelegation(delegationID)
	if err != nil {
		return err
	}
	if del == nil {
		return NewReverts("delegation is empty")
	}
	if del.LastIteration != nil {
		return NewReverts("delegation is already signaled exit")
	}
	if del.Stake == 0 {
		return NewReverts("delegation has already been withdrawn")
	}

	// there can never be a delegation pointing to a non-existent validation
	// if the validation does not exist it's a system error
	val, err := s.validationService.GetExistingValidation(del.Validation)
	if err != nil {
		return err
	}

	// ensure delegation can be signaled ( delegation has started and has not ended )
	started, err := del.Started(val, currentBlock)
	if err != nil {
		return err
	}
	if !started {
		return NewReverts("delegation has not started yet, funds can be withdrawn")
	}
	ended, err := del.Ended(val, currentBlock)
	if err != nil {
		return err
	}
	if ended {
		return NewReverts("delegation has ended, funds can be withdrawn")
	}

	current, err := val.CurrentIteration(currentBlock)
	if err != nil {
		return err
	}
	if err = s.delegationService.SignalExit(del, delegationID, current); err != nil {
		logger.Info("signal delegation exit failed", "delegationID", delegationID, "error", err)
		return err
	}

	err = s.aggregationService.SignalExit(del.Validation, del.WeightedStake())
	if err != nil {
		return err
	}

	if val.Status == validation.StatusActive {
		if err = s.validationService.AddToRenewalList(del.Validation); err != nil {
			return err
		}
	}

	logger.Info("signal delegation exit", "delegationID", delegationID)
	return nil
}

// WithdrawDelegation allows expired and queued delegations to withdraw their stake.
func (s *Staker) WithdrawDelegation(
	delegationID *big.Int,
	currentBlock uint32,
) (uint64, error) {
	logger.Debug("withdrawing delegation", "delegationID", delegationID)

	del, err := s.delegationService.GetDelegation(delegationID)
	if err != nil {
		return 0, err
	}

	if del == nil {
		return 0, NewReverts("delegation is empty")
	}

	// there can never be a delegation pointing to a non-existent validation
	// if the validation does not exist it's a system error
	val, err := s.validationService.GetExistingValidation(del.Validation)
	if err != nil {
		return 0, err
	}

	// ensure the delegation is either queued or finished
	started, err := del.Started(val, currentBlock)
	if err != nil {
		return 0, err
	}
	finished, err := del.Ended(val, currentBlock)
	if err != nil {
		return 0, err
	}
	if started && !finished {
		return 0, NewReverts("delegation is not eligible for withdraw")
	}

	// withdraw delegation
	withdrawableStake, err := s.delegationService.Withdraw(del, delegationID)
	if err != nil {
		logger.Info("failed to withdraw", "delegationID", delegationID, "error", err)
		return 0, err
	}

	// start and finish values are sanitized: !started and finished is impossible
	// delegation is still queued
	if !started && val.Status <= validation.StatusActive {
		weightedStake := stakes.NewWeightedStakeWithMultiplier(withdrawableStake, del.Multiplier)
		if err = s.aggregationService.SubPendingVet(del.Validation, weightedStake); err != nil {
			return 0, err
		}

		if err = s.globalStatsService.RemoveQueued(withdrawableStake); err != nil {
			return 0, err
		}
	} else {
		if err = s.globalStatsService.RemoveWithdrawable(withdrawableStake); err != nil {
			return 0, err
		}
	}

	if err = s.ContractBalanceCheck(withdrawableStake); err != nil {
		return 0, err
	}

	logger.Info("withdrew delegation", "delegationID", delegationID, "stake", withdrawableStake)

	return withdrawableStake, nil
}

// IncreaseDelegatorsReward Increases reward for validation's delegators.
func (s *Staker) IncreaseDelegatorsReward(node thor.Address, reward *big.Int, currentBlock uint32) error {
	return s.validationService.IncreaseDelegatorsReward(node, reward, currentBlock)
}

func (s *Staker) validateStakeIncrease(validator thor.Address, validation *validation.Validation, amount uint64) error {
	if amount > MaxStakeVET {
		return NewReverts("increase amount is too large")
	}
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

	total, err := checkStake(valNextPeriodTVL, aggNextPeriodTVL, amount)
	if err != nil {
		return err
	}

	if total > MaxStakeVET {
		return NewReverts("total stake would exceed maximum")
	}

	return nil
}

func checkStake(valNextPeriodTVL, aggNextPeriodTVL, amount uint64) (uint64, error) {
	total1, carry := bits.Add64(valNextPeriodTVL, aggNextPeriodTVL, 0)
	total2, carry := bits.Add64(total1, amount, carry)
	if carry != 0 {
		return 0, NewReverts("stake is out of range")
	}
	return total2, nil
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

func (s *Staker) getValidationOrRevert(valID thor.Address) (*validation.Validation, error) {
	val, err := s.validationService.GetValidation(valID)
	if err != nil {
		return nil, err
	}
	if val == nil {
		return nil, NewReverts("validation does not exist")
	}
	return val, nil
}

// GetEffectiveVET returns the EffectiveVET in the contract
func (s *Staker) GetEffectiveVET() (uint64, error) {
	// accessing slot 0 defined in staker.sol
	val, err := s.state.GetStorage(s.address, thor.Bytes32{})
	if err != nil {
		return 0, err
	}

	return ToVET(new(big.Int).SetBytes(val.Bytes())), nil
}
