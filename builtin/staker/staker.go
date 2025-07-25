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
	"github.com/vechain/thor/v2/builtin/staker/globalstats"
	"github.com/vechain/thor/v2/builtin/staker/renewal"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// TODO: Do these need to be set in params.sol, or some other dynamic way?
var (
	logger   = log.WithContext("pkg", "staker")
	MinStake = big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	MaxStake = big.NewInt(0).Mul(big.NewInt(600e6), big.NewInt(1e18))

	LowStakingPeriod    = uint32(360) * 24 * 7  // 336 epochs
	MediumStakingPeriod = uint32(360) * 24 * 15 // 720 epochs
	HighStakingPeriod   = uint32(360) * 24 * 30 // 1,440 epochs

	cooldownPeriod = uint32(8640)
	epochLength    = uint32(180)
)

func SetLogger(l log.Logger) {
	logger = l
}

// Staker implements native methods of `Staker` contract.
type Staker struct {
	delegations *delegations
	validations *validations
	storage     *storage
	params      *params.Params

	aggregationService *aggregation.Service
	globalStatsService *globalstats.Service
}

// New create a new instance.
func New(addr thor.Address, state *state.State, params *params.Params, charger *gascharger.Charger) *Staker {
	sctx := solidity.NewContext(addr, state, charger)
	storage := newStorage(addr, state, charger)

	// debug overrides for testing
	storage.debugOverride(&epochLength, slotEpochLength)
	storage.debugOverride(&cooldownPeriod, slotCooldownPeriod)

	return &Staker{
		aggregationService: aggregation.New(sctx),
		globalStatsService: globalstats.New(sctx),
		storage:            storage,
		validations:        newValidations(storage),
		delegations:        newDelegations(storage),
		params:             params,
	}
}

// IsPoSActive checks if the staker contract has become active, i.e. we have transitioned to PoS.
func (s *Staker) IsPoSActive() (bool, error) {
	return s.validations.IsActive()
}

// LeaderGroup lists all registered candidates.
func (s *Staker) LeaderGroup() (map[thor.Address]*Validation, error) {
	return s.validations.LeaderGroup()
}

// LockedVET returns the amount of VET and weight locked by validations and delegations.
func (s *Staker) LockedVET() (*big.Int, *big.Int, error) {
	return s.globalStatsService.GetLocketVET()
}

// QueuedStake returns the amount of VET and weight queued by validations and delegations.
func (s *Staker) QueuedStake() (*big.Int, *big.Int, error) {
	return s.globalStatsService.GetQueuedStake()
}

// FirstActive returns validator address of first entry.
func (s *Staker) FirstActive() (*thor.Address, error) {
	return s.validations.FirstActive()
}

// FirstQueued returns validator address of first entry.
func (s *Staker) FirstQueued() (*thor.Address, error) {
	return s.validations.FirstQueued()
}

// QueuedGroupSize returns the number of validations in the queue
func (s *Staker) QueuedGroupSize() (*big.Int, error) {
	return s.validations.validatorQueue.Len()
}

func (s *Staker) LeaderGroupSize() (*big.Int, error) {
	return s.validations.leaderGroup.Len()
}

// Next returns the next validator in a linked list.
// If the provided ID is not in a list, it will return empty bytes.
func (s *Staker) Next(prev thor.Address) (thor.Address, error) {
	entry, err := s.Get(prev)
	if err != nil {
		return thor.Address{}, err
	}
	if entry.IsEmpty() || entry.Next == nil {
		return thor.Address{}, nil
	}
	return *entry.Next, nil
}

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
	if err := s.validations.Add(endorsor, node, period, stake); err != nil {
		logger.Info("add validator failed", "node", node, "error", err)
		return err
	}

	// update global totals
	err := s.globalStatsService.AddQueued(stake, big.NewInt(0).Mul(stake, validatorWeightMultiplier))
	if err != nil {
		return err
	}

	logger.Info("added validator", "node", node)
	return nil
}

func (s *Staker) Get(id thor.Address) (*Validation, error) {
	return s.storage.GetValidation(id)
}

func (s *Staker) SignalExit(endorsor thor.Address, id thor.Address) error {
	logger.Debug("signal exit", "endorsor", endorsor, "id", id)

	if err := s.validations.SignalExit(endorsor, id); err != nil {
		logger.Info("signal exit failed", "id", id, "error", err)
		return err
	}

	return nil
}

// IncreaseStake increases the stake of a queued or active validator
// if a validator is active, the stake is increase, but the weight stays the same
// the weight will be recalculated at the end of the staking period, by the housekeep function
func (s *Staker) IncreaseStake(endorsor thor.Address, validationID thor.Address, amount *big.Int) error {
	logger.Debug("increasing stake", "endorsor", endorsor, "validationID", validationID, "amount", new(big.Int).Div(amount, big.NewInt(1e18)))
	if err := s.validations.IncreaseStake(validationID, endorsor, amount); err != nil {
		logger.Info("increase stake failed", "validationID", validationID, "error", err)
		return err
	}

	// validate that new TVL is <= Max stake
	nextPeriodTVL, err := s.nextPeriodTVL(validationID)
	if err != nil {
		return err
	}
	if nextPeriodTVL.Cmp(MaxStake) > 0 {
		return errors.New("stake is out of range")
	}

	// update global totals
	if err = s.globalStatsService.AddQueued(amount, big.NewInt(0).Mul(amount, validatorWeightMultiplier)); err != nil {
		return err
	}

	logger.Info("increased stake", "validationID", validationID)
	return nil
}

func (s *Staker) DecreaseStake(endorsor thor.Address, validationID thor.Address, amount *big.Int) error {
	amountETH := new(big.Int).Div(amount, big.NewInt(1e18))
	logger.Debug("decreasing stake", "endorsor", endorsor, "validationID", validationID, "amount", amountETH)

	if err := s.validations.DecreaseStake(validationID, endorsor, amount); err != nil {
		logger.Info("decrease stake failed", "validationID", validationID, "error", err)
		return err
	}

	// remove global queued values
	validation, err := s.storage.GetValidation(validationID)
	if err != nil {
		return err
	}
	if validation.Status == StatusQueued {
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

	// remove global queued values
	// TODO shift this around - the issue here is the state change of the validator from Queued -> Exit after withdrawal
	validation, err := s.storage.GetValidation(validationID)
	if err != nil {
		return nil, err
	}
	initialValidationStatus := validation.Status

	stake, err := s.validations.WithdrawStake(endorsor, validationID, currentBlock)
	if err != nil {
		logger.Info("withdraw failed", "validationID", validationID, "error", err)
		return nil, err
	}

	// remove global queued values
	// TODO connecting with the above TODO
	if initialValidationStatus == StatusQueued {
		err = s.globalStatsService.RemoveQueued(validation.PendingLocked, big.NewInt(0).Mul(validation.PendingLocked, validatorWeightMultiplier))
		if err != nil {
			return nil, err
		}
	}

	logger.Info("withdrew validator staker", "validationID", validationID)
	return stake, nil
}

// GetWithdrawable returns the withdrawable stake of a validator.
func (s *Staker) GetWithdrawable(id thor.Address, block uint32) (*big.Int, error) {
	_, stake, err := s.validations.GetWithdrawable(id, block)
	return stake, err
}

func (s *Staker) SetOnline(id thor.Address, online bool) (bool, error) {
	logger.Debug("set node online", "id", id, "online", online)
	entry, err := s.storage.GetValidation(id)
	if err != nil {
		return false, err
	}
	hasChanged := entry.Online != online
	entry.Online = online
	if hasChanged {
		err = s.storage.SetValidation(id, entry, false)
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

	// add delegation
	delegationID, err := s.delegations.Add(validationID, stake, multiplier)
	if err != nil {
		logger.Info("failed to add delegation", "ValidationID", validationID, "error", err)
		return thor.Bytes32{}, err
	}

	// update delegation aggregations
	// TODO use service
	delegation, err := s.storage.GetDelegation(delegationID)
	if err != nil {
		return thor.Bytes32{}, err
	}
	if err = s.aggregationService.AddPendingVET(validationID, delegation.CalcWeight(), stake); err != nil {
		return thor.Bytes32{}, err
	}

	// validate that new TVL is <= Max stake
	nextPeriodTVL, err := s.nextPeriodTVL(validationID)
	if err != nil {
		return thor.Bytes32{}, err
	}
	if nextPeriodTVL.Cmp(MaxStake) > 0 {
		return thor.Bytes32{}, errors.New("validation's next period stake exceeds max stake")
	}

	// update global figures
	if err = s.globalStatsService.AddQueued(stake, delegation.CalcWeight()); err != nil {
		return thor.Bytes32{}, err
	}

	logger.Info("added delegation", "ValidationID", validationID, "delegationID", delegationID)
	return delegationID, nil
}

// GetDelegation returns the delegation.
func (s *Staker) GetDelegation(
	delegationID thor.Bytes32,
) (*Delegation, *Validation, error) {
	delegation, err := s.storage.GetDelegation(delegationID)
	if err != nil {
		return nil, nil, err
	}
	if delegation.IsEmpty() {
		return &Delegation{}, nil, nil
	}
	validation, err := s.storage.GetValidation(delegation.ValidationID)
	if err != nil {
		return nil, nil, err
	}
	return delegation, validation, nil
}

// HasDelegations returns true if the validator has any delegations.
func (s *Staker) HasDelegations(
	node thor.Address,
) (bool, error) {
	_, err := s.storage.GetValidation(node)
	if err != nil {
		return false, err
	}

	agg, err := s.aggregationService.GetAggregation(node)
	if err != nil {
		return false, err
	}

	return !agg.IsEmpty(), nil
}

// SignalDelegationExit updates the auto-renewal status of a delegation.
func (s *Staker) SignalDelegationExit(delegationID thor.Bytes32) error {
	logger.Debug("updating autorenew", "delegationID", delegationID)

	if err := s.delegations.SignalExit(delegationID); err != nil {
		logger.Info("update autorenew failed", "delegationID", delegationID, "error", err)
		return err
	}

	delegation, err := s.storage.GetDelegation(delegationID)
	if err != nil {
		return err
	}

	err = s.aggregationService.SignalExit(delegation.ValidationID)
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

	// withdraw from delegation
	started, finished, withdrawableStake, withdrawableStakeWeight, err := s.delegations.Withdraw(delegationID)
	if err != nil {
		logger.Info("failed to withdraw", "delegationID", delegationID, "error", err)
		return nil, err
	}
	// start and finish values are sanitized: !started and finished is impossible

	// update the aggregation
	delegation, err := s.storage.GetDelegation(delegationID)
	if err != nil {
		return nil, err
	}

	if !started { // delegation's funds are still pending
		if err := s.aggregationService.SubPendingVet(delegation.ValidationID, withdrawableStake, withdrawableStakeWeight); err != nil {
			return nil, err
		}

		if err := s.globalStatsService.RemoveQueued(withdrawableStake, withdrawableStakeWeight); err != nil {
			return nil, err
		}
	}

	if finished { // delegation's funds have move to withdrawable
		if err = s.aggregationService.SubWithdrawableVET(delegation.ValidationID, withdrawableStake); err != nil {
			return nil, err
		}
	}

	logger.Info("withdrew delegation", "delegationID", delegationID, "stake", new(big.Int).Div(withdrawableStake, big.NewInt(1e18)))
	return withdrawableStake, nil
}

// GetDelegatorRewards returns reward amount for validation id and staking period.
func (s *Staker) GetDelegatorRewards(validationID thor.Address, stakingPeriod uint32) (*big.Int, error) {
	return s.storage.GetDelegatorRewards(validationID, stakingPeriod)
}

// GetCompletedPeriods returns number of completed staking periods for validation.
func (s *Staker) GetCompletedPeriods(validationID thor.Address) (uint32, error) {
	return s.storage.GetCompletedPeriods(validationID)
}

// IncreaseDelegatorsReward Increases reward for validation's delegators.
func (s *Staker) IncreaseDelegatorsReward(node thor.Address, reward *big.Int) error {
	return s.storage.IncreaseDelegatorsReward(node, reward)
}

// GetValidatorsTotals returns the total stake, total weight, total delegators stake and total delegators weight.
func (s *Staker) GetValidatorsTotals(validationID thor.Address) (*ValidationTotals, error) {
	validator, err := s.storage.GetValidation(validationID)
	if err != nil {
		return nil, err
	}
	agg, err := s.aggregationService.GetAggregation(validationID)
	if err != nil {
		return nil, err
	}
	return &ValidationTotals{
		TotalLockedStake:        new(big.Int).Add(validator.LockedVET, agg.LockedVET),
		TotalLockedWeight:       new(big.Int).Set(validator.Weight),
		DelegationsLockedStake:  new(big.Int).Set(agg.LockedVET),
		DelegationsLockedWeight: new(big.Int).Set(agg.LockedWeight),
	}, nil
}

func (s *Staker) nextPeriodTVL(id thor.Address) (*big.Int, error) {
	validator, err := s.storage.GetValidation(id)
	if err != nil {
		return nil, err
	}
	validatorTVL := validator.NextPeriodTVL()

	agg, err := s.aggregationService.GetAggregation(id)
	if err != nil {
		return nil, err
	}

	return big.NewInt(0).Add(validatorTVL, agg.NextPeriodTVL()), nil
}

func (s *Staker) ActivateNextValidator(currentBlk uint32, maxLeaderGroupSize *big.Int) (*thor.Address, error) {
	validatorID, validator, err := s.validations.NextToActivate(maxLeaderGroupSize)
	if err != nil {
		return nil, err
	}
	logger.Debug("activating validator", "validatorID", validatorID, "block", currentBlk)

	aggRenew, err := s.aggregationService.Renew(*validatorID)
	if err != nil {
		return nil, err
	}

	// update the validator values
	// TODO move this to the validatorservice at some point
	validatorLocked := big.NewInt(0).Add(validator.LockedVET, validator.PendingLocked)
	validator.PendingLocked = big.NewInt(0)
	validator.LockedVET = validatorLocked
	// x2 multiplier for validator's stake
	validatorWeight := big.NewInt(0).Mul(validatorLocked, validatorWeightMultiplier)
	validator.Weight = big.NewInt(0).Add(validatorWeight, aggRenew.ChangeWeight)

	// update the validator statuses
	validator.Status = StatusActive
	validator.Online = true
	validator.StartBlock = currentBlk
	// add to the active list
	added, err := s.validations.leaderGroup.Add(*validatorID, validator)
	if err != nil {
		return nil, err
	}
	if !added {
		return nil, errors.New("failed to add validator to active list")
	}

	validatorRenewal := &renewal.Renewal{
		ChangeTVL:            validator.LockedVET,
		ChangeWeight:         validator.Weight,
		QueuedDecrease:       validator.LockedVET,
		QueuedDecreaseWeight: validator.Weight,
	}
	if err = s.globalStatsService.UpdateTotals(validatorRenewal, aggRenew); err != nil {
		return nil, err
	}

	return validatorID, nil
}
