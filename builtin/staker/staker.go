// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"

	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/builtin/params"
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
}

// New create a new instance.
func New(addr thor.Address, state *state.State, params *params.Params, charger *gascharger.Charger) *Staker {
	storage := newStorage(addr, state, charger)

	// debug overrides for testing
	storage.debugOverride(&epochLength, slotEpochLength)
	storage.debugOverride(&cooldownPeriod, slotCooldownPeriod)

	return &Staker{
		storage:     storage,
		validations: newValidations(storage),
		delegations: newDelegations(storage),
		params:      params,
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
	lockedVet, err := s.storage.lockedVET.Get()
	if err != nil {
		return nil, nil, err
	}

	lockedWeight, err := s.storage.lockedWeight.Get()
	return lockedVet, lockedWeight, err
}

// QueuedStake returns the amount of VET and weight queued by validations and delegations.
func (s *Staker) QueuedStake() (*big.Int, *big.Int, error) {
	queuedVet, err := s.storage.queuedVET.Get()
	if err != nil {
		return nil, nil, err
	}

	queuedWeight, err := s.storage.queuedWeight.Get()
	return queuedVet, queuedWeight, err
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

	if err := s.validations.Add(endorsor, node, period, stake); err != nil {
		logger.Info("add validator failed", "node", node, "error", err)
		return err
	} else {
		logger.Info("added validator", "node", node)
		return nil
	}
}

func (s *Staker) Get(id thor.Address) (*Validation, error) {
	return s.storage.GetValidation(id)
}

func (s *Staker) SignalExit(endorsor thor.Address, id thor.Address) error {
	logger.Debug("signal exit", "endorsor", endorsor, "id", id)

	if err := s.validations.SignalExit(endorsor, id); err != nil {
		logger.Info("signal exit failed", "id", id, "error", err)
		return err
	} else {
		logger.Info("signal", "id", id)
		return nil
	}
}

// IncreaseStake increases the stake of a queued or active validator
// if a validator is active, the stake is increase, but the weight stays the same
// the weight will be recalculated at the end of the staking period, by the housekeep function
func (s *Staker) IncreaseStake(endorsor thor.Address, id thor.Address, amount *big.Int) error {
	amountETH := new(big.Int).Div(amount, big.NewInt(1e18))
	logger.Debug("increasing stake", "endorsor", endorsor, "id", id, "amount", amountETH)
	if err := s.validations.IncreaseStake(id, endorsor, amount); err != nil {
		logger.Info("increase stake failed", "id", id, "error", err)
		return err
	} else {
		logger.Info("increased stake", "id", id)
		return nil
	}
}

func (s *Staker) DecreaseStake(endorsor thor.Address, id thor.Address, amount *big.Int) error {
	amountETH := new(big.Int).Div(amount, big.NewInt(1e18))
	logger.Debug("decreasing stake", "endorsor", endorsor, "id", id, "amount", amountETH)

	if err := s.validations.DecreaseStake(id, endorsor, amount); err != nil {
		logger.Info("decrease stake failed", "id", id, "error", err)
		return err
	} else {
		logger.Info("decreased stake", "id", id)
		return nil
	}
}

// WithdrawStake allows expired validations to withdraw their stake.
func (s *Staker) WithdrawStake(endorsor thor.Address, id thor.Address, currentBlock uint32) (*big.Int, error) {
	logger.Debug("withdrawing stake", "endorsor", endorsor, "id", id)

	stake, err := s.validations.WithdrawStake(endorsor, id, currentBlock)
	if err != nil {
		logger.Info("withdraw failed", "id", id, "error", err)
	} else {
		logger.Info("withdrew validator staker", "id", id)
	}
	return stake, err
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
	autoRenew bool,
	multiplier uint8,
) (thor.Bytes32, error) {
	stakeETH := new(big.Int).Div(stake, big.NewInt(1e18))
	logger.Debug("adding delegation", "ValidationID", validationID, "stake", stakeETH, "autoRenew", autoRenew, "multiplier", multiplier)
	if id, err := s.delegations.Add(validationID, stake, autoRenew, multiplier); err != nil {
		logger.Info("failed to add delegation", "ValidationID", validationID, "error", err)
		return thor.Bytes32{}, err
	} else {
		logger.Info("added delegation", "ValidationID", validationID, "id", id)
		return id, nil
	}
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
	aggregation, err := s.storage.GetAggregation(node)
	if err != nil {
		return false, err
	}
	if aggregation == nil || aggregation.IsEmpty() {
		return false, nil
	}
	total := new(big.Int).Add(aggregation.CurrentRecurringVET, aggregation.CurrentOneTimeVET)
	return total.Sign() > 0, nil
}

// UpdateDelegationAutoRenew updates the auto-renewal status of a delegation.
func (s *Staker) UpdateDelegationAutoRenew(
	delegationID thor.Bytes32,
	autoRenew bool,
) error {
	logger.Debug("updating autorenew", "delegationID", delegationID, "autoRenew", autoRenew)
	var err error
	if autoRenew {
		err = s.delegations.EnableAutoRenew(delegationID)
	} else {
		err = s.delegations.DisableAutoRenew(delegationID)
	}
	if err != nil {
		logger.Info("update autorenew failed", "delegationID", delegationID, "error", err)
	} else {
		logger.Info("updated autorenew", "delegationID", delegationID)
	}
	return err
}

// WithdrawDelegation allows expired and queued delegations to withdraw their stake.
func (s *Staker) WithdrawDelegation(
	delegationID thor.Bytes32,
) (*big.Int, error) {
	logger.Debug("withdrawing delegation", "delegationID", delegationID)
	if stake, err := s.delegations.Withdraw(delegationID); err != nil {
		logger.Info("failed to withdraw", "delegationID", delegationID, "error", err)
		return nil, err
	} else {
		stakeETH := new(big.Int).Div(stake, big.NewInt(1e18))
		logger.Info("withdrew delegation", "delegationID", delegationID, "stake", stakeETH)
		return stake, nil
	}
}

// GetRewards returns reward amount for validation id and staking period.
func (s *Staker) GetRewards(validationID thor.Address, stakingPeriod uint32) (*big.Int, error) {
	return s.storage.GetRewards(validationID, stakingPeriod)
}

// GetCompletedPeriods returns number of completed staking periods for validation.
func (s *Staker) GetCompletedPeriods(validationID thor.Address) (uint32, error) {
	return s.storage.GetCompletedPeriods(validationID)
}

// IncreaseReward Increases reward for node address, for current staking period.
func (s *Staker) IncreaseReward(node thor.Address, reward big.Int) error {
	return s.storage.IncreaseReward(node, reward)
}

// GetValidatorsTotals returns the total stake, total weight, total delegators stake and total delegators weight.
func (s *Staker) GetValidatorsTotals(validationID thor.Address) (*ValidationTotals, error) {
	validator, err := s.storage.GetValidation(validationID)
	if err != nil {
		return nil, err
	}
	aggregation, err := s.storage.GetAggregation(validationID)
	if err != nil {
		return nil, err
	}
	delegationLockedStake := big.NewInt(0).Add(aggregation.CurrentRecurringVET, aggregation.CurrentOneTimeVET)
	return &ValidationTotals{
		TotalLockedStake:        big.NewInt(0).Add(validator.LockedVET, delegationLockedStake),
		TotalLockedWeight:       validator.Weight,
		DelegationsLockedStake:  big.NewInt(0).Add(aggregation.CurrentRecurringVET, aggregation.CurrentOneTimeVET),
		DelegationsLockedWeight: big.NewInt(0).Add(aggregation.CurrentRecurringWeight, aggregation.CurrentOneTimeWeight),
	}, nil
}
