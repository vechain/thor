// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"bytes"
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/aggregation"
	"github.com/vechain/thor/v2/builtin/staker/delegation"
	"github.com/vechain/thor/v2/builtin/staker/globalstats"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// TODO: Do these need to be set in params.sol, or some other dynamic way?
var (
	logger                    = log.WithContext("pkg", "staker")
	MinStake                  = big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	MaxStake                  = big.NewInt(0).Mul(big.NewInt(600e6), big.NewInt(1e18))
	validatorWeightMultiplier = big.NewInt(2)

	LowStakingPeriod    = newConfigVar("staker-low-staking-period", 360*24*7)     // 7 Days
	MediumStakingPeriod = newConfigVar("staker-medium-staking-period", 360*24*15) // 15 Days
	HighStakingPeriod   = newConfigVar("staker-high-staking-period", 360*24*30)   // 30 Days

	CooldownPeriod = newConfigVar("cooldown-period", 8640) // 8640 blocks, 1 day
	EpochLength    = newConfigVar("epoch-length", 180)     // 180 epochs
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

	validatorWeightMultiplier *big.Int
}

// New create a new instance.
func New(addr thor.Address, state *state.State, params *params.Params, charger *gascharger.Charger) *Staker {
	sctx := solidity.NewContext(addr, state, charger)

	// debug overrides for testing
	debugOverride(sctx, LowStakingPeriod)
	debugOverride(sctx, MediumStakingPeriod)
	debugOverride(sctx, HighStakingPeriod)
	debugOverride(sctx, EpochLength)
	debugOverride(sctx, CooldownPeriod)

	return &Staker{
		params:                    params,
		validatorWeightMultiplier: validatorWeightMultiplier,

		aggregationService: aggregation.New(sctx),
		globalStatsService: globalstats.New(sctx),
		delegationService:  delegation.New(sctx),
		validationService: validation.New(
			sctx,
			validatorWeightMultiplier,
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
func (s *Staker) Get(id thor.Address) (*validation.Validation, error) {
	return s.validationService.GetValidation(id)
}

// GetWithdrawable returns the withdrawable stake of a validator.
func (s *Staker) GetWithdrawable(id thor.Address, block uint32) (*big.Int, error) {
	val, err := s.validationService.GetExistingValidation(id)
	if err != nil {
		return nil, err
	}

	return val.CalculateWithdrawableVET(block, CooldownPeriod.Get()), err
}

// GetDelegation returns the delegation.
func (s *Staker) GetDelegation(
	delegationID thor.Bytes32,
) (*delegation.Delegation, *validation.Validation, error) {
	del, err := s.delegationService.GetDelegation(delegationID)
	if err != nil {
		return nil, nil, err
	}
	if del.IsEmpty() {
		return &delegation.Delegation{}, nil, nil
	}
	val, err := s.validationService.GetValidation(del.ValidationID)
	if err != nil {
		return nil, nil, err
	}
	return del, val, nil
}

// HasDelegations returns true if the validator has any delegations.
func (s *Staker) HasDelegations(
	node thor.Address,
) (bool, error) {
	_, err := s.validationService.GetValidation(node)
	if err != nil {
		return false, err
	}

	agg, err := s.aggregationService.GetAggregation(node)
	if err != nil {
		return false, err
	}

	return !agg.IsEmpty(), nil
}

// GetDelegatorRewards returns reward amount for validation id and staking period.
func (s *Staker) GetDelegatorRewards(validationID thor.Address, stakingPeriod uint32) (*big.Int, error) {
	return s.validationService.GetDelegatorRewards(validationID, stakingPeriod)
}

// GetCompletedPeriods returns number of completed staking periods for validation.
func (s *Staker) GetCompletedPeriods(validationID thor.Address) (uint32, error) {
	return s.validationService.GetCompletedPeriods(validationID)
}

// GetValidatorsTotals returns the total stake, total weight, total delegators stake and total delegators weight.
func (s *Staker) GetValidatorsTotals(validationID thor.Address) (*validation.ValidationTotals, error) {
	validator, err := s.validationService.GetValidation(validationID)
	if err != nil {
		return nil, err
	}
	agg, err := s.aggregationService.GetAggregation(validationID)
	if err != nil {
		return nil, err
	}
	return &validation.ValidationTotals{
		TotalLockedStake:        new(big.Int).Add(validator.LockedVET, agg.LockedVET),
		TotalLockedWeight:       new(big.Int).Set(validator.Weight),
		DelegationsLockedStake:  new(big.Int).Set(agg.LockedVET),
		DelegationsLockedWeight: new(big.Int).Set(agg.LockedWeight),
	}, nil
}

// Next returns the next validator in a linked list.
// If the provided ID is not in a list, it will return empty bytes.
func (s *Staker) Next(prev thor.Address) (thor.Address, error) {
	entry, err := s.validationService.GetValidation(prev)
	if err != nil {
		return thor.Address{}, err
	}
	if entry.IsEmpty() || entry.Next == nil {
		return thor.Address{}, nil
	}
	return *entry.Next, nil
}

//
// Setters - state change
//

// AddValidator queues a new validator.
func (s *Staker) AddValidator(
	endorsor thor.Address,
	node thor.Address,
	period uint32,
	stake *big.Int,
) error {
	logger.Debug("adding validator", "endorsor", endorsor,
		"node", node,
		"period", period,
		"stake", new(big.Int).Div(stake, big.NewInt(1e18)),
	)

	// create a new validation
	if err := s.validationService.Add(endorsor, node, period, stake); err != nil {
		logger.Info("add validator failed", "node", node, "error", err)
		return err
	}

	// update global totals
	err := s.globalStatsService.AddQueued(stake, big.NewInt(0).Mul(stake, s.validatorWeightMultiplier))
	if err != nil {
		return err
	}

	logger.Info("added validator", "node", node)
	return nil
}

func (s *Staker) SignalExit(endorsor thor.Address, id thor.Address) error {
	logger.Debug("signal exit", "endorsor", endorsor, "id", id)

	if err := s.validationService.SignalExit(endorsor, id); err != nil {
		logger.Info("signal exit failed", "id", id, "error", err)
		return err
	}

	return nil
}

// IncreaseStake increases the stake of a queued or active validator
// if a validator is active, the QueuedVET is increase, but the weight stays the same
// the weight will be recalculated at the end of the staking period, by the housekeep function
func (s *Staker) IncreaseStake(endorsor thor.Address, validationID thor.Address, amount *big.Int) error {
	logger.Debug("increasing stake", "endorsor", endorsor, "validationID", validationID, "amount", new(big.Int).Div(amount, big.NewInt(1e18)))

	if err := s.validationService.IncreaseStake(validationID, endorsor, amount); err != nil {
		logger.Info("increase stake failed", "validationID", validationID, "error", err)
		return err
	}

	// validate that new TVL is <= Max stake
	if err := s.validateNextPeriodTVL(validationID); err != nil {
		return err
	}

	// update global totals
	if err := s.globalStatsService.AddQueued(amount, big.NewInt(0).Mul(amount, validatorWeightMultiplier)); err != nil {
		return err
	}

	logger.Info("increased stake", "validationID", validationID)
	return nil
}

func (s *Staker) DecreaseStake(endorsor thor.Address, validationID thor.Address, amount *big.Int) error {
	logger.Debug("decreasing stake", "endorsor", endorsor, "validationID", validationID, "amount", new(big.Int).Div(amount, big.NewInt(1e18)))

	if err := s.validationService.DecreaseStake(validationID, endorsor, amount); err != nil {
		logger.Info("decrease stake failed", "validationID", validationID, "error", err)
		return err
	}

	// remove queued VET from the global stats if validator is queued
	val, err := s.validationService.GetValidation(validationID)
	if err != nil {
		return err
	}
	if val.Status == validation.StatusQueued {
		err = s.globalStatsService.RemoveQueued(amount, big.NewInt(0).Mul(amount, validatorWeightMultiplier))
		if err != nil {
			return err
		}
	}

	logger.Info("decreased stake", "validationID", validationID)
	return nil
}

// WithdrawStake allows expired validations to withdraw their stake.
func (s *Staker) WithdrawStake(endorsor thor.Address, validationID thor.Address, currentBlock uint32) (*big.Int, error) {
	logger.Debug("withdrawing stake", "endorsor", endorsor, "validationID", validationID)

	// remove validator QueuedVET if the validator is still queued
	val, err := s.validationService.GetValidation(validationID)
	if err != nil {
		return nil, err
	}
	if val.Status == validation.StatusQueued {
		err = s.globalStatsService.RemoveQueued(val.QueuedVET, big.NewInt(0).Mul(val.QueuedVET, validatorWeightMultiplier))
		if err != nil {
			return nil, err
		}
	}

	stake, err := s.validationService.WithdrawStake(endorsor, validationID, currentBlock)
	if err != nil {
		logger.Info("withdraw failed", "validationID", validationID, "error", err)
		return nil, err
	}

	logger.Info("withdrew validator staker", "validationID", validationID)
	return stake, nil
}

func (s *Staker) SetOnline(id thor.Address, online bool) (bool, error) {
	logger.Debug("set node online", "id", id, "online", online)
	entry, err := s.validationService.GetValidation(id)
	if err != nil {
		return false, err
	}
	hasChanged := entry.Online != online
	entry.Online = online
	if hasChanged {
		err = s.validationService.SetValidation(id, entry, false)
	} else {
		err = nil
	}
	return hasChanged, err
}

// AddDelegation adds a new delegation.
func (s *Staker) AddDelegation(
	validationID thor.Address,
	stake *big.Int,
	multiplier uint8,
) (thor.Bytes32, error) {
	logger.Debug("adding delegation", "ValidationID", validationID, "stake", new(big.Int).Div(stake, big.NewInt(1e18)), "multiplier", multiplier)

	// ensure validation is ok to receive a new delegation
	val, err := s.validationService.GetExistingValidation(validationID)
	if err != nil {
		return thor.Bytes32{}, err
	}

	if val.Status != validation.StatusQueued && val.Status != validation.StatusActive {
		return thor.Bytes32{}, errors.New("validation is not queued or active")
	}

	// add delegation on the next iteration - val.CurrentIteration() + 1,
	delegationID, err := s.delegationService.Add(validationID, val.CurrentIteration()+1, stake, multiplier)
	if err != nil {
		logger.Info("failed to add delegation", "ValidationID", validationID, "error", err)
		return thor.Bytes32{}, err
	}

	// update delegation aggregations
	// TODO use service + cleanup multiple calls
	del, err := s.delegationService.GetDelegation(delegationID)
	if err != nil {
		return thor.Bytes32{}, err
	}
	if err = s.aggregationService.AddPendingVET(validationID, del.CalcWeight(), stake); err != nil {
		return thor.Bytes32{}, err
	}

	// validate that new TVL is <= Max stake
	if err := s.validateNextPeriodTVL(validationID); err != nil {
		return thor.Bytes32{}, err
	}

	// update global figures
	if err = s.globalStatsService.AddQueued(stake, del.CalcWeight()); err != nil {
		return thor.Bytes32{}, err
	}

	logger.Info("added delegation", "ValidationID", validationID, "delegationID", delegationID)
	return delegationID, nil
}

// SignalDelegationExit updates the auto-renewal status of a delegation.
func (s *Staker) SignalDelegationExit(delegationID thor.Bytes32) error {
	logger.Debug("updating autorenew", "delegationID", delegationID)

	del, err := s.delegationService.GetDelegation(delegationID)
	if err != nil {
		return err
	}

	if del.IsEmpty() {
		return errors.New("delegation is empty")
	}

	val, err := s.validationService.GetValidation(del.ValidationID)
	if err != nil {
		return err
	}

	if err := s.delegationService.SignalExit(delegationID, val); err != nil {
		logger.Info("update autorenew failed", "delegationID", delegationID, "error", err)
		return err
	}

	// Calculate the specific delegation's stake and weight
	delegationWeight := del.CalcWeight()

	err = s.aggregationService.SignalExit(del.ValidationID, del.Stake, delegationWeight)
	if err != nil {
		return err
	}

	logger.Info("updated autorenew", "delegationID", delegationID)
	return nil
}

// WithdrawDelegation allows expired and queued delegations to withdraw their stake.
func (s *Staker) WithdrawDelegation(
	delegationID thor.Bytes32,
) (*big.Int, error) {
	logger.Debug("withdrawing delegation", "delegationID", delegationID)

	// todo refactor to make less calls to the repo
	del, err := s.delegationService.GetDelegation(delegationID)
	if err != nil {
		return nil, err
	}

	val, err := s.validationService.GetValidation(del.ValidationID)
	if err != nil {
		return nil, err
	}

	started := del.Started(val)
	finished := del.Ended(val)
	if started && !finished {
		return nil, errors.New("delegation is not eligible for withdraw")
	}

	// withdraw from delegation
	withdrawableStake, withdrawableStakeWeight, err := s.delegationService.Withdraw(delegationID)
	if err != nil {
		logger.Info("failed to withdraw", "delegationID", delegationID, "error", err)
		return nil, err
	}
	// start and finish values are sanitized: !started and finished is impossible

	// update the aggregation
	del, err = s.delegationService.GetDelegation(delegationID)
	if err != nil {
		return nil, err
	}

	if !started { // delegation's funds are still pending
		if err := s.aggregationService.SubPendingVet(del.ValidationID, withdrawableStake, withdrawableStakeWeight); err != nil {
			return nil, err
		}

		if err := s.globalStatsService.RemoveQueued(withdrawableStake, withdrawableStakeWeight); err != nil {
			return nil, err
		}
	}

	if finished { // delegation's funds have move to withdrawable
		if err = s.aggregationService.SubWithdrawableVET(del.ValidationID, withdrawableStake); err != nil {
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

func (s *Staker) validateNextPeriodTVL(id thor.Address) error {
	validator, err := s.validationService.GetValidation(id)
	if err != nil {
		return err
	}

	agg, err := s.aggregationService.GetAggregation(id)
	if err != nil {
		return err
	}

	// accumulated TVL should cannot be more than MaxStake
	if big.NewInt(0).Add(validator.NextPeriodTVL(), agg.NextPeriodTVL()).Cmp(MaxStake) > 0 {
		return errors.New("stake is out of range")
	}

	return nil
}

type configVar struct {
	slot        thor.Bytes32
	value       uint32
	initialised bool
}

func newConfigVar(name string, defaultValue uint32) *configVar {
	return &configVar{
		slot:  thor.BytesToBytes32([]byte(name)),
		value: defaultValue,
	}
}

func (c *configVar) init(value uint32) {
	c.initialised = true
	c.value = value
}

func (c *configVar) Get() uint32 {
	return c.value
}

func (c *configVar) Name() string {
	return string(bytes.TrimLeft(c.slot[:], "0"))
}

func (c *configVar) Slot() thor.Bytes32 {
	return c.slot
}

func debugOverride(sctx *solidity.Context, config *configVar) {
	if config.initialised { // early return to prevent subsequent reads
		return
	}
	num, err := solidity.NewUint256(sctx, config.slot).Get()
	if err != nil {
		logger.Warn("failed to read config value", "slot", config.Name(), "error", err)
		return
	}
	config.initialised = true

	if num.Uint64() != 0 {
		config.value = uint32(num.Uint64())
		logger.Debug("debug override found new config value", "slot", config.Name(), "value", config.Get())
	} else {
		logger.Debug("using default config value", "slot", config.Name(), "value", config.value)
	}
}
